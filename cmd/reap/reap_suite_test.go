package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestReap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Reap Suite")
}
