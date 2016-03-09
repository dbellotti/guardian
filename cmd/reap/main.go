package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func main() {
	pidFile := flag.String("pid-file", "", "")
	flag.Parse()

	if os.Args[1] == "exit" {
		os.Exit(99)
	}

	if flag.Args()[0] == "test" {
		cmd := exec.Command(os.Args[0], "exit")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags: syscall.CLONE_PARENT,
		}

		check(cmd.Start())
		fmt.Println("PID", cmd.Process.Pid)
		check(ioutil.WriteFile(*pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0700))
		return
	}

	cmd := exec.Command(flag.Args()[0], flag.Args()[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	signals := make(chan os.Signal, 100)
	signal.Notify(signals, syscall.SIGCHLD)

	var err error
	if err = cmd.Start(); err != nil {
		panic(err)
	}

	fd3 := os.NewFile(3, "/proc/self/fd/3")

	pid := -2
	for range signals {
		fmt.Println("about to reap")
		var status syscall.WaitStatus
		var rusage syscall.Rusage

		wpid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, &rusage)
		if err != nil {
			fmt.Println(err)
		}

		if wpid == cmd.Process.Pid {
			pid = readPid(*pidFile)
			check(err)

			fmt.Println("read pid: ", pid)
			fd3.Write([]byte{byte(status.ExitStatus())})
		}

		if wpid == pid {
			os.Exit(status.ExitStatus())
		}

		fmt.Println("REAP", wpid, status.ExitStatus())
	}
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func readPid(pidFile string) (pid int) {
	b, err := ioutil.ReadFile(pidFile)
	check(err)

	if _, err := fmt.Sscanf(string(b), "%d", &pid); err != nil {
		check(err)
	}

	return pid
}
