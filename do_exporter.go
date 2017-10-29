package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"time"

	"github.com/digitalocean/godo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"golang.org/x/oauth2"
)

const (
	nameSpace = "digital_ocean"
	subSystem = "account"
)

var (
	listenAddress = flag.String("listen-address", ":8080", "Address on which to expose metrics.")
	metricsPath   = flag.String("telemetry-path", "/metrics", "Path under which to expose metrics.")
)

type Config struct {
	doToken string
}

func (t *Config) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: t.doToken}, nil
}

type AccountCollector struct {
	client                                               *godo.Client
	timeout                                              time.Duration
	active, dropletLimit, emailVerified, floatingIPLimit prometheus.Gauge
}

func (ac *AccountCollector) Describe(ch chan<- *prometheus.Desc) {
	ac.active.Describe(ch)
	ac.dropletLimit.Describe(ch)
	ac.emailVerified.Describe(ch)
	ac.floatingIPLimit.Describe(ch)
}

func (ac *AccountCollector) Collect(ch chan<- prometheus.Metric) {

	ac.reset()

	ctx, clCtx := context.WithTimeout(context.Background(), ac.timeout)
	defer clCtx()

	resp, _, err := ac.client.Account.Get(ctx)
	if err != nil {
		log.Errorf("Can't get a valid response: %v", err)

		return
	}

	if resp.Status == "active" {
		ac.active.Set(1)
	}
	ch <- ac.active

	ac.dropletLimit.Set(float64(resp.DropletLimit))
	ch <- ac.dropletLimit

	if resp.EmailVerified {
		ac.emailVerified.Set(1)
	}
	ch <- ac.emailVerified

	ac.floatingIPLimit.Set(float64(resp.FloatingIPLimit))
	ch <- ac.floatingIPLimit

}

func (ac *AccountCollector) reset() {
	ac.active.Set(0)
	ac.dropletLimit.Set(0)
	ac.emailVerified.Set(0)
	ac.floatingIPLimit.Set(0)
}

func NewAccountCollector(client *godo.Client) *AccountCollector {
	return &AccountCollector{
		client:  client,
		timeout: 3 * time.Second,
		active: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: nameSpace,
			Subsystem: subSystem,
			Name:      "active",
			Help:      "if 1 account is active",
		}),
		dropletLimit: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: nameSpace,
			Subsystem: subSystem,
			Name:      "droplet_limit",
			Help:      "total number of droplets you can create",
		}),
		emailVerified: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: nameSpace,
			Subsystem: subSystem,
			Name:      "email_verified",
			Help:      "if 1 email was verified",
		}),
		floatingIPLimit: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: nameSpace,
			Subsystem: subSystem,
			Name:      "floating_ip_limit",
			Help:      "total number of floating IPs that you can have",
		}),
	}
}

func main() {
	flag.Parse()

	log.Infoln("Starting do_exporter")

	token := os.Getenv("DO_TOKEN")

	if token == "" {
		log.Error("missing DO_TOKEN env variable")
		os.Exit(1)
	}

	doConfig := &Config{
		doToken: token,
	}
	oauthClient := oauth2.NewClient(context.Background(), doConfig)
	doClient := godo.NewClient(oauthClient)

	prometheus.MustRegister(NewAccountCollector(doClient))
	prometheus.Unregister(prometheus.NewGoCollector())
	prometheus.Unregister(prometheus.NewProcessCollector(os.Getgid(), ""))

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, *metricsPath, http.StatusMovedPermanently)
	})

	log.Fatal(http.ListenAndServe(*listenAddress, nil))

}
