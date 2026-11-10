package cmd_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

func TestCmd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ccloud")
}

var example cloud.Cloud

var _ = BeforeSuite(func() {
	example = cloud.Cloud{
		Name:         "example",
		Issuer:       "https://issuer.example.test",
		ClientID:     "client.example.test",
		ClientSecret: "public-desktop-secret",
		Scopes:       []string{"email"},
		APIServer:    "https://api.example.test",
		Regions: []cloud.Region{
			{Name: "north1", Host: "k8s.north1.example.test", Port: 17001, Authority: authority()},
			{Name: "south1", Host: "k8s.south1.example.test", Port: 17002, Authority: authority()},
		},
	}
})

func authority() []byte {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "example-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
