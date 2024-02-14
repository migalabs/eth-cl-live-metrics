package exporter

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	// TODO: Just hardcoded, move to config
	EndpointUrl string = "metrics"

	MetricLoopInterval time.Duration = 15 * time.Second
)

type PrometheusMetrics struct {
	ctx context.Context

	ExposedIp       string
	ExposedPort     string
	EndpointUrl     string
	RefreshInterval time.Duration

	Modules []*MetricsModule

	wg     sync.WaitGroup
	closeC chan struct{}
}

func NewPrometheusMetrics(ctx context.Context, ip string, port int) *PrometheusMetrics {
	return &PrometheusMetrics{
		ctx:             ctx,
		ExposedIp:       ip,
		ExposedPort:     fmt.Sprintf("%d", port),
		EndpointUrl:     EndpointUrl,
		RefreshInterval: MetricLoopInterval,
		Modules:         make([]*MetricsModule, 0),
		closeC:          make(chan struct{}),
	}
}

func (p *PrometheusMetrics) AddMetricsModule(newMod *MetricsModule) {
	p.Modules = append(p.Modules, newMod)
}

func (p *PrometheusMetrics) Start() error {
	http.Handle("/"+p.EndpointUrl, promhttp.Handler())
	go func() {
		log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%s", p.ExposedIp, p.ExposedPort), nil))
	}()
	log.Infof("prometheus metrics listening on: %s:%s", p.ExposedIp, p.ExposedPort)
	err := p.initPrometheusMetrics()
	if err != nil {
		return errors.Wrap(err, "unable to init prometheus metrics")
	}

	p.wg.Add(1)
	go p.launchMetricsUpdater()

	return nil
}

func (p *PrometheusMetrics) initPrometheusMetrics() error {
	log.Debugf("initializing %d metrics modules", len(p.Modules))
	// iter through all the available modules - and call the
	// mudule.InitMetrics() method
	for _, mod := range p.Modules {
		err := mod.Init()
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PrometheusMetrics) launchMetricsUpdater() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.RefreshInterval)

metricsUpdateLoop:
	for {
		select {
		case <-ticker.C:
			log.Trace("updating values for prometheus metrics")
			// update all the submodules on prometheus
			for _, mod := range p.Modules {
				summary := make(map[string]interface{}, 0)
				modSum := mod.UpdateSummary()
				for key, value := range modSum {
					summary[key] = value
				}
				// compose a message with the give summary
				logFields := log.Fields(modSum)
				log.WithFields(logFields).Debugf("summary for %s", mod.Name())
			}

		case <-p.closeC:
			log.Debug("detected a controled shutdown")
			break metricsUpdateLoop
		case <-p.ctx.Done():
			log.Debug("detected that context died, shutting down")
			break metricsUpdateLoop
		}
	}
}

func (p *PrometheusMetrics) Close() {
	// Init loop for each of the Exporters
	log.Infof("closing %d prometheus metrics modules", len(p.Modules))
	p.closeC <- struct{}{}
	p.wg.Wait()
	log.Infof("prometheus metrics exporte successfully closed")
}
