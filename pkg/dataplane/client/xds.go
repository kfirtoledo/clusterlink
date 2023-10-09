package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/clusterlink-net/clusterlink/pkg/dataplane/server"
)

// resources indicate the xDS resources that would be fetched
var resources = [...]string{resource.ClusterType, resource.ListenerType}

// XDSClient implements the client which fetches clusters and listeners
type XDSClient struct {
	dataplane          *server.Dataplane
	controlplaneTarget string
	tlsConfig          *tls.Config
	errors             map[string]error
	logger             *logrus.Entry
}

func (x *XDSClient) runFetcher(resourceType string) error {
	for {
		conn, err := grpc.Dial(x.controlplaneTarget, grpc.WithTransportCredentials(credentials.NewTLS(x.tlsConfig)))
		if err != nil {
			x.logger.Errorf("Failed to dial controlplane xDS server: %v.", err)
			time.Sleep(5 * time.Second)
			continue
		}
		x.logger.Infof("Successfully connected to the controlplane xDS server.")

		f, err := newFetcher(context.Background(), conn, resourceType, x.dataplane)
		if err != nil {
			x.logger.Errorf("Failed to initialize fetcher: %v.", err)
			time.Sleep(5 * time.Second)
			continue
		}
		x.logger.Infof("Successfully initialized client for %s type.", resourceType)
		err = f.Run()
		x.logger.Infof("Fetcher '%s' stopped: %v.", resourceType, err)
		time.Sleep(5 * time.Second)
	}
}

// Run starts the running xDS client which fetches clusters and listeners from the controlplane.
func (x *XDSClient) Run() error {
	var wg sync.WaitGroup
	wg.Add(len(resources))
	for _, res := range resources {
		go func(res string) {
			defer wg.Done()
			err := x.runFetcher(res)
			x.logger.Errorf("Fetcher (%s) stopped: %v", res, err)
			x.errors[res] = err
		}(res)
	}
	wg.Wait()

	var errs []error
	for resource, err := range x.errors {
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"error running fetcher '%s': %v", resource, err))
		}
	}

	return errors.Join(errs...)
}

// NewXDSClient returns am xDS client which can fetch clusters and listeners from the controlplane.
func NewXDSClient(dataplane *server.Dataplane, controlplaneTarget string, tlsConfig *tls.Config) *XDSClient {
	return &XDSClient{dataplane: dataplane,
		controlplaneTarget: controlplaneTarget,
		tlsConfig:          tlsConfig,
		errors:             make(map[string]error),
		logger:             logrus.WithField("component", "xds.client")}
}
