package dns_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/cloudfoundry-incubator/guardian/kawasaki/dns"
	"github.com/docker/docker/pkg/reexec"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
)

func init() {
	if reexec.Init() {
		os.Exit(0)
	}
}

var _ = Describe("RootfsWriter", func() {
	var (
		rootfsPath string

		rootfsWriter *dns.RootfsWriter
		rootUid      int
		rootGid      int

		log lager.Logger
	)

	BeforeEach(func() {
		rootUid = 40000
		rootGid = 40001
	})

	JustBeforeEach(func() {
		rootfsWriter = &dns.RootfsWriter{
			RootfsPath: rootfsPath,
			RootUid:    rootUid,
			RootGid:    rootGid,
		}

		log = lagertest.NewTestLogger("test")
	})

	Describe("WriteFile", func() {
		Context("when the root path exists", func() {
			BeforeEach(func() {
				var err error

				rootfsPath, err = ioutil.TempDir("", "rootfs")
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				Expect(os.RemoveAll(rootfsPath)).To(Succeed())
			})

			It("should write the file there", func() {
				Expect(rootfsWriter.WriteFile(log, "/test/file.txt", []byte("Hello world"))).To(Succeed())

				filePath := filepath.Join(rootfsPath, "test/file.txt")
				Expect(filePath).To(BeARegularFile())

				f, err := os.Open(filePath)
				Expect(err).NotTo(HaveOccurred())
				contents, err := ioutil.ReadAll(f)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(contents)).To(Equal("Hello world"))
			})

			It("should apply the correct ownership", func() {
				filePath := filepath.Join(rootfsPath, "/test/file.txt")
				Expect(rootfsWriter.WriteFile(log, "/test/file.txt", []byte("Hello world"))).To(Succeed())

				stat, err := os.Stat(filePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(stat.Sys().(*syscall.Stat_t).Uid).To(BeEquivalentTo(40000))
				Expect(stat.Sys().(*syscall.Stat_t).Gid).To(BeEquivalentTo(40001))
			})

			Context("when the file path is a symlink", func() {
				var (
					target string
				)

				BeforeEach(func() {
					var err error
					target, err = ioutil.TempDir("", "symlink")
					Expect(err).NotTo(HaveOccurred())

					Expect(os.Symlink(target, filepath.Join(rootfsPath, "symlink"))).To(Succeed())
					Expect(os.MkdirAll(filepath.Join(rootfsPath, target), 0700)).To(Succeed())
				})

				It("is resolved relative to the root path", func() {
					Expect(rootfsWriter.WriteFile(log, "/symlink/file.txt", []byte("Hello world"))).To(Succeed())
					Expect(filepath.Join(target, "file.txt")).NotTo(BeAnExistingFile())
				})
			})
		})
	})
})
