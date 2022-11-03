package cmd

import (
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/tdahar/block-scorer/pkg/app"
	"github.com/tdahar/block-scorer/pkg/utils"
	cli "github.com/urfave/cli/v2"
)

var AnalyzerCommand = &cli.Command{
	Name:   "block-scorer",
	Usage:  "Receive Block proposals from clients and evaluate score",
	Action: LaunchBlockAnalyzer,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "log-level",
			Usage:       "info,debug,warn",
			DefaultText: "info",
		},
		&cli.StringFlag{
			Name:        "bn-endpoints",
			Usage:       "beacon node endpoints (label/endpoint,label/endpoint)",
			DefaultText: "lh/localhost:5052",
		},
		&cli.StringFlag{
			Name:        "db-endpoint",
			Usage:       "postgresql database endpoint: postgresql://user:password@localhost:5432/beaconchain",
			DefaultText: "lh/localhost:5052",
		},
		&cli.StringFlag{
			Name:        "db-workers",
			Usage:       "10",
			DefaultText: "1",
		}},
}

var QueryTimeout = 90 * time.Second

func LaunchBlockAnalyzer(c *cli.Context) error {
	dbWorkers := 1
	logLauncher := log.WithField(
		"module", "ScorerCommand",
	)

	logLauncher.Info("parsing flags")
	if !c.IsSet("log-level") {
		logLauncher.Infof("Setting log level to Info")
		logrus.SetLevel(logrus.InfoLevel)
	} else {
		logrus.SetLevel(utils.ParseLogLevel(c.String("log-level")))
	}
	// check if a beacon node is set
	if !c.IsSet("bn-endpoints") {
		return errors.New("bn endpoint not provided")
	}

	if !c.IsSet("db-endpoint") {
		return errors.New("db endpoint not provided")
	}

	if !c.IsSet("db-workers") {
		logLauncher.Warnf("no database workers configured, default is 1")
	} else {
		dbWorkers = c.Int("db-workers")
	}

	bnEndpoints := strings.Split(c.String("bn-endpoints"), ",")
	dbEndpoint := c.String("db-endpoint")

	service, err := app.NewAppService(c.Context, bnEndpoints, dbEndpoint, dbWorkers)
	if err != nil {
		log.Fatal("could not start app: %s", err.Error())
	}
	service.Run()

	return nil
}