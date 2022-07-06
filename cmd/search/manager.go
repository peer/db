package main

import (
	"crypto/tls"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
)

const (
	certificateReloadInterval = 24 * time.Hour
)

type CertificateManager struct {
	CertFile    string
	KeyFile     string
	Log         zerolog.Logger
	certificate *tls.Certificate
	mu          sync.RWMutex
	ticker      *time.Ticker
	done        chan bool
}

func (c *CertificateManager) Start() errors.E {
	err := c.reloadCertificate()
	if err != nil {
		return err
	}
	c.ticker = time.NewTicker(certificateReloadInterval)
	c.done = make(chan bool)
	go func() {
		for {
			select {
			case <-c.done:
				return
			case <-c.ticker.C:
				err := c.reloadCertificate()
				if err != nil {
					c.Log.Error().Err(err).Fields(errors.AllDetails(err)).Str("certFile", c.CertFile).Str("keyFile", c.KeyFile).Send()
				}
			}
		}
	}()
	return nil
}

func (c *CertificateManager) reloadCertificate() errors.E {
	certificate, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return errors.WithStack(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.certificate = &certificate
	return nil
}

func (c *CertificateManager) Stop() {
	c.ticker.Stop()
	c.done <- true
}

func (c *CertificateManager) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.certificate, nil
}
