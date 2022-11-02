package analysis

import (
	"context"
	"fmt"
	"time"

	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/jackc/pgx/v4"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/sirupsen/logrus"
	"github.com/tdahar/block-scorer/pkg/analysis/additional_structs"
	"github.com/tdahar/block-scorer/pkg/client_api"
	"github.com/tdahar/block-scorer/pkg/postgresql"
)

var (
	moduleName = "Analysis"
	log        = logrus.WithField(
		"module", moduleName)
)

type ClientLiveData struct {
	ctx              context.Context
	Eth2Provider     client_api.APIClient                                       // connection to the beacon node
	AttHistory       map[phase0.Slot]map[phase0.CommitteeIndex]bitfield.Bitlist // 32 slots of attestation per slot and committeeIndex
	BlockRootHistory map[phase0.Slot]phase0.Root                                // 64 slots of roots
	log              *logrus.Entry                                              // each analyzer has its own logger
	ProcessNewHead   chan struct{}
	DBClient         *postgresql.PostgresDBService
	writeBatch       pgx.Batch
	EpochData        additional_structs.EpochStructs
}

func NewBlockAnalyzer(ctx context.Context, label string, cliEndpoint string, timeout time.Duration, dbClient *postgresql.PostgresDBService) (*ClientLiveData, error) {
	client, err := client_api.NewAPIClient(ctx, label, cliEndpoint, timeout)
	if err != nil {
		log.Errorf("could not create eth2 client: %s", err)
		return &ClientLiveData{}, err
	}
	return &ClientLiveData{
		ctx:              ctx,
		Eth2Provider:     *client,
		DBClient:         dbClient,
		AttHistory:       make(map[phase0.Slot]map[phase0.CommitteeIndex]bitfield.Bitlist),
		BlockRootHistory: make(map[phase0.Slot]phase0.Root),
		log:              log.WithField("label", label),
		writeBatch:       pgx.Batch{},
		EpochData:        additional_structs.NewEpochData(client.Api),
	}, nil
}

// Asks for a block proposal to the client and stores score in the database
func (b *ClientLiveData) ProcessNewBlock(slot phase0.Slot) error {
	log := b.log.WithField("task", "generate-block")
	log.Debugf("processing new block: %d\n", slot)
	randaoReveal := phase0.BLSSignature{}
	graffiti := []byte("")
	snapshot := time.Now()
	block, err := b.Eth2Provider.Api.BeaconBlockProposal(b.ctx, slot, randaoReveal, graffiti) // ask for block proposal
	blockTime := time.Since(snapshot).Seconds()                                               // time to generate block
	if err != nil {
		return fmt.Errorf("error requesting block from %s: %s", b.Eth2Provider.Label, err)

	}
	for i := range b.AttHistory {
		if i+32 < slot { // attestations can only reference 32 slots back
			delete(b.AttHistory, i) // remove old entries from the map
		}
	}

	for i := range b.BlockRootHistory {
		if i+64 < slot { // attestations can only reference 32 slots back
			delete(b.BlockRootHistory, i) // remove old entries from the map
		}
	}

	// for now we just have Bellatrix
	metrics, err := b.BellatrixBlockMetrics(block.Bellatrix)
	if err != nil {
		return fmt.Errorf("error analyzing block from %s: %s", b.Eth2Provider.Label, err)
	}
	log.Infof("Block Generation Time: %f", blockTime)
	log.Infof("Metrics: %+v", metrics)
	b.DBClient.InsertNewScore(metrics)

	// We block the update attestations as new head could impact attestations of the proposed block
	b.ProcessNewHead <- struct{}{} // Allow the new head to update attestations
	return nil
}
