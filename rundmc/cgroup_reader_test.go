package rundmc_test

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/cloudfoundry-incubator/guardian/rundmc"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/specs"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("CgroupReader", func() {
	var (
		cgroupsPath string

		cgroupReader rundmc.CgroupReader

		logger lager.Logger
	)

	BeforeEach(func() {
		var err error

		cgroupsPath, err = ioutil.TempDir("", "")
		Expect(err).NotTo(HaveOccurred())

		cgroupCpuSharePath := path.Join(cgroupsPath, "cpu", "test")
		Expect(os.MkdirAll(cgroupCpuSharePath, 0744)).To(Succeed())
		Expect(ioutil.WriteFile(
			path.Join(cgroupCpuSharePath, "cpu.shares"), []byte("512"), 0744,
		)).To(Succeed())

		logger = lagertest.NewTestLogger("test")

		cgroupReader = rundmc.NewCgroupReader(cgroupsPath)
	})

	AfterEach(func() {
		Expect(os.RemoveAll(cgroupsPath)).To(Succeed())
	})

	Describe("CPUCgroup", func() {
		It("should return cpu share for container", func() {
			expectedCpuShare := uint64(512)
			expectedResult := specs.CPU{
				Shares: &expectedCpuShare,
			}

			actualShare, _ := cgroupReader.CPUCgroup(logger, "test")

			Expect(actualShare).To(Equal(expectedResult))
		})
	})
})
