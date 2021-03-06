package bundlerules

import (
	"github.com/cloudfoundry-incubator/goci"
	"github.com/cloudfoundry-incubator/guardian/gardener"
)

type Env struct {
}

func (r Env) Apply(bndl *goci.Bndl, spec gardener.DesiredContainerSpec) *goci.Bndl {
	process := bndl.Process()
	process.Env = spec.Env
	return bndl.WithProcess(process)
}
