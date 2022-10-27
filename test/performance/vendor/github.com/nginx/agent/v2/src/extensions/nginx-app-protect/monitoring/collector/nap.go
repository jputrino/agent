package collector

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"gopkg.in/mcuadros/go-syslog.v2"

	"github.com/nginx/agent/v2/src/extensions/nginx-app-protect/monitoring"
)

const (
	napComponentName = "collector:nap"
)

var (
	// logging fields for the component
	componentLogFields = logrus.Fields{
		"component": napComponentName,
	}
)

// NAPCollector lets you to Collect log data on given port.
type NAPCollector struct {
	syslog *syslogServer
	logger *logrus.Entry
}

type syslogServer struct {
	channel syslog.LogPartsChannel
	handler *syslog.ChannelHandler
	server  *syslog.Server
}

// NewNAPCollector gives you a NAP collector for the syslog server.
func NewNAPCollector(cfg *NAPConfig) (*NAPCollector, error) {
	var (
		c   NAPCollector
		err error
	)

	c.logger = logrus.StandardLogger().WithFields(componentLogFields)
	if cfg.Logger != nil {
		c.logger = cfg.Logger.WithFields(componentLogFields)
	}
	c.logger.Infof("Getting %s Collector", monitoring.NAP)

	c.syslog, err = newSyslogServer(c.logger, cfg.SyslogIP, cfg.SyslogPort)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func newSyslogServer(logger *logrus.Entry, ip string, port int) (*syslogServer, error) {
	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)

	server := syslog.NewServer()
	server.SetFormat(syslog.RFC3164)
	server.SetHandler(handler)

	addr := fmt.Sprintf("%s:%d", ip, port)
	err := server.ListenTCP(addr)
	if err != nil {
		msg := fmt.Sprintf("Error while configuring syslog server to listen on %s:\n %v", addr, err)
		logger.Error(msg)
		return nil, err
	}

	err = server.Boot()
	if err != nil {
		msg := fmt.Sprintf("Error while booting the syslog server at %s:\n %v ", addr, err)
		logger.Error(msg)
		return nil, err
	}

	return &syslogServer{channel, handler, server}, nil
}

// Collect starts collecting on collect chan until done chan gets a signal.
func (nap *NAPCollector) Collect(ctx context.Context, wg *sync.WaitGroup, collect chan<- *monitoring.RawLog) {
	defer wg.Done()

	nap.logger.Infof("Starting collection for %s", monitoring.NAP)

	for {
		select {
		case logParts := <-nap.syslog.channel:
			line, ok := logParts["content"].(string)
			if !ok {
				nap.logger.Warnf("Noncompliant syslog message, got: %v", logParts)
				break
			}

			nap.logger.Infof("collected log line succesfully.")
			collect <- &monitoring.RawLog{Origin: monitoring.NAP, Logline: line}
		case <-ctx.Done():
			nap.logger.Infof("Context cancellation, collector is wrapping up...")

			err := nap.syslog.server.Kill()
			if err != nil {
				nap.logger.Errorf("Error while killing syslog collector server: %v", err)
			}

			return
		}
	}
}