package main

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

var (
	certFile = flag.String("cert", "", "A PEM encoded certificate file.")
	keyFile  = flag.String("key", "", "A PEM encoded private key file (if not in the cert PEM file).")
	caFile   = flag.String("CA", "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", "A PEM encoded CA's certificate file.")
)

var tlsConfig *tls.Config
var caCertPool = x509.NewCertPool()

func loadTLS() {
	// Load CA cert
	if *verbose {
		log.Println("Loading CA certs")
	}
	err := LoadCertficatesFromFile(*caFile)
	if err != nil {
		log.Fatal(err)
	}

	// Setup HTTPS client
	tlsConfig = &tls.Config{
		RootCAs:            caCertPool,
		ClientCAs:          caCertPool,
		InsecureSkipVerify: false,
		//ClientAuth:         tls.RequireAndVerifyClientCert,
		Renegotiation: tls.RenegotiateOnceAsClient,

		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		},
	}

	// Load client cert
	if *certFile != "" {
		// If cert and key are the same file, as the key is not specified
		if *keyFile == "" {
			keyFile = certFile
		}

		if *verbose {
			log.Println("Loading client cert", *certFile, "and key", *keyFile)
		}
		cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
		if err == nil {
			// Load the certificate into the leaf
			if cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0]); err != nil {
				log.Fatal(err)
			}
			if *verbose {
				log.Println("Loaded certificate, Sub:", certPKIXString(cert.Leaf.Subject, ","), "Issuer: ", certPKIXString(cert.Leaf.Issuer, ","))
			}

			if chains, err := cert.Leaf.Verify(x509.VerifyOptions{Roots: caCertPool}); err != nil {
				log.Fatal("Unable to verify provided cert with the provided CA", err, chains)
			}

			tlsConfig.Certificates = []tls.Certificate{cert}
		}
	}

	if *verbose {
		log.Println("Assigning HTTP default client")
	}
	http.DefaultClient = &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
		//Timeout:   60 * time.Minute,
	}

}

func certPKIXString(name pkix.Name, sep string) (out string) {
	for i := len(name.Names) - 1; i >= 0; i-- {
		//fmt.Println(name.Names[i])
		if out != "" {
			out += sep
		}
		out += pkix.RDNSequence([]pkix.RelativeDistinguishedNameSET{name.Names[i : i+1]}).String()
	}
	return
}

func LoadCertficatesFromFile(path string) error {
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	for {
		block, rest := pem.Decode(raw)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				fmt.Println("warning: error parsing CA cert", err)
				continue
			}
			//fmt.Printf("cert: %s  %#v\n", cert.Subject, cert.Subject)

			if *verbose {
				sub, is := certPKIXString(cert.Subject, ","), certPKIXString(cert.Issuer, ",")
				if sub != is {
					fmt.Printf("  Adding CA: %s (%02x) from %s\n", sub, cert.SerialNumber, is)
				} else {
					fmt.Printf("  Adding CA: %s (%02x)\n", sub, cert.SerialNumber)
				}
			}
			caCertPool.AddCert(cert)
		}
		raw = rest
	}

	return nil
}
