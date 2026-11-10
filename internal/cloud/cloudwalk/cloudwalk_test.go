package cloudwalk_test

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/internal/cloud/cloudwalk"
)

func TestCloudwalk(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cloudwalk")
}

var _ = Describe("the cloudwalk cloud", func() {
	It("pins a CA certificate for every region and gives each its own tunnel port", func() {
		ports := map[int]string{}
		for _, region := range cloudwalk.Cloud().Regions {
			block, _ := pem.Decode(region.Authority)
			Expect(block).NotTo(BeNil(), region.Name)
			certificate, err := x509.ParseCertificate(block.Bytes)
			Expect(err).NotTo(HaveOccurred(), region.Name)
			Expect(certificate.IsCA).To(BeTrue(), region.Name)
			Expect(ports).NotTo(HaveKey(region.Port), region.Name+" shares a tunnel port")
			ports[region.Port] = region.Name
		}
	})
})
