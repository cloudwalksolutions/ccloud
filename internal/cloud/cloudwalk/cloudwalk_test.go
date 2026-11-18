package cloudwalk_test

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
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

	It("asks each region's own dev gateway, over HTTPS", func() {
		for _, region := range cloudwalk.Cloud().Regions {
			Expect(region.Gateway).To(Equal("https://api."+region.Name+".cloudwalksolutions.net"), region.Name)
		}
	})

	It("learns of newer releases from ccloud's own GitHub releases", func() {
		Expect(cloudwalk.Cloud().Releases).To(Equal("https://api.github.com/repos/cloudwalksolutions/ccloud/releases/latest"))
	})

	It("finds an app repo's file under .cloudwalk, beside its workflows", func() {
		Expect(cloudwalk.Cloud().AppDir()).To(Equal(".cloudwalk"))
	})

	It("tunnels to the leader of each environment's Postgres cluster, as its owner, the one secret developers may read", func() {
		Expect(cloudwalk.Cloud().Database).To(Equal(cloud.Database{
			Primary: "postgres-operator.crunchydata.com/cluster=crunchydb,postgres-operator.crunchydata.com/role=master",
			Port:    5432,
			Secret:  "crunchydb-pguser-crunchydb",
			Reset:   "db-reset",
		}))
	})
})
