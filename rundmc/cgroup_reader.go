package rundmc

import (
	"fmt"
	"io/ioutil"
	"path"
	"strconv"

	"github.com/opencontainers/specs"
	"github.com/pivotal-golang/lager"
)

type cgroupReader struct {
	cgroupsPath string
}

func NewCgroupReader(cgroupsPath string) CgroupReader {
	return &cgroupReader{
		cgroupsPath: cgroupsPath,
	}
}

func (c *cgroupReader) CPUCgroup(log lager.Logger, handle string) (specs.CPU, error) {
	contents, err := ioutil.ReadFile(path.Join(c.cgroupsPath, "cpu", handle, "cpu.shares"))
	fmt.Println(err)
	shares, _ := strconv.ParseUint(string(contents), 10, 64)

	return specs.CPU{
		Shares: &shares,
	}, nil
}
