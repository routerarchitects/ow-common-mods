package servicediscovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/routerarchitects/ra-common-mods/buildinfo"
	"github.com/routerarchitects/ra-common-mods/kafka"
)

var log *slog.Logger

var ErrSingletonAlreadyCreated = errors.New("discovery instance already created")

// Discovery manages consuming and publishing discovery events.
type Discovery struct {
	dcfg Config
	kcfg kafka.Config

	self Instance

	store *store

	producer kafka.Producer
	consumer kafka.Consumer

	ctx    context.Context
	cancel context.CancelFunc

	wg sync.WaitGroup
}

var (
	singletonMu sync.Mutex
	singleton   *Discovery
)

func New(dcfg Config, kcfg kafka.Config, logger *slog.Logger) (*Discovery, error) {
	singletonMu.Lock()
	defer singletonMu.Unlock()

	if singleton != nil {
		return nil, ErrSingletonAlreadyCreated
	}

	if logger == nil {
		logger = slog.Default()
	}
	if err := validateConfig(dcfg); err != nil {
		return nil, err
	}

	id := dcfg.InstanceID
	key := dcfg.InstanceKey
	if id == 0 {
		id = mustRandInt64()
	}
	if key == "" {
		key = sha256Hex(dcfg.PublicEndpoint)
	}

	self := Instance{
		ID:              id,
		Key:             key,
		Type:            dcfg.ServiceType,
		Version:         GetServiceVersion(),
		PrivateEndPoint: dcfg.PrivateEndpoint,
		PublicEndPoint:  dcfg.PublicEndpoint,
		LastSeenUTC:     time.Now().UTC(),
	}

	if kcfg.Consumer.GroupID == "" {
		kcfg.Consumer.GroupID = fmt.Sprintf("%s-discovery-%d", dcfg.ServiceType, id)
	}

	st := newStore(self.PrivateEndPoint, self.ID, dcfg.Ordering)
	log = logger

	singleton = &Discovery{
		dcfg:  dcfg,
		kcfg:  kcfg,
		self:  self,
		store: st,
	}

	return singleton, nil
}

func GetServiceVersion() string {
	version := buildinfo.GetVersion()
	commitHash := buildinfo.GetCommitHash()
	if version == "" {
		version = commitHash
	} else {
		version = fmt.Sprintf("%s-%s", version, commitHash)
	}

	return version
}

func validateConfig(cfg Config) error {
	var errs []error

	if strings.TrimSpace(cfg.Topic) == "" {
		errs = append(errs, errors.New("topic is required"))
	}
	if strings.TrimSpace(cfg.ServiceType) == "" {
		errs = append(errs, errors.New("service type is required"))
	}
	if strings.TrimSpace(GetServiceVersion()) == "" {
		errs = append(errs, errors.New("service version is required"))
	}
	if strings.TrimSpace(cfg.PrivateEndpoint) == "" {
		errs = append(errs, errors.New("private endpoint is required"))
	}
	if strings.TrimSpace(cfg.PublicEndpoint) == "" {
		errs = append(errs, errors.New("public endpoint is required"))
	}
	if cfg.KeepAliveInterval <= 0 {
		errs = append(errs, errors.New("keep-alive interval must be greater than zero"))
	}
	if cfg.ExpiryMultiplier < 1 {
		errs = append(errs, errors.New("expiry multiplier must be at least 1"))
	}
	if cfg.SweepInterval <= 0 {
		errs = append(errs, errors.New("sweep interval must be greater than zero"))
	}

	switch cfg.Ordering {
	case OrderingRoundRobin, OrderingLastSeen, OrderingLatestVersion, OrderingNone:
	default:
		errs = append(errs, fmt.Errorf("invalid ordering strategy: %q", cfg.Ordering))
	}

	return errors.Join(errs...)
}

// Store returns the local discovery store.
func (d *Discovery) Store() Store { return d.store }

// Self returns this instance's advertised identity.
func (d *Discovery) Self() Instance { return d.self }

// Start starts the consumer, publisher and expiry sweeper.
func (d *Discovery) Start(parent context.Context) error {
	if parent == nil {
		parent = context.Background()
	}
	if d.ctx != nil {
		return errors.New("discovery already started")
	}

	prod, err := kafka.NewProducer(d.kcfg)
	if err != nil {
		return err
	}
	cons, err := kafka.NewConsumer(d.kcfg, log)
	if err != nil {
		_ = prod.Close()
		return err
	}
	d.producer = prod
	d.consumer = cons
	d.ctx, d.cancel = context.WithCancel(parent)

	// Start consumer loop.
	d.wg.Go(func() {
		err := d.consumer.Subscribe(d.ctx, d.dcfg.Topic, d.handleMessage, nil)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error("discovery consumer exited", "error", err)
		}
	})

	// Start publisher loop.
	d.wg.Go(func() {
		d.publisherLoop(d.ctx)
	})

	// Start sweeper.
	d.wg.Go(func() {
		d.sweeperLoop(d.ctx)
	})

	return nil
}

func (d *Discovery) GetInstances() *Discovery {
	singletonMu.Lock()
	defer singletonMu.Unlock()
	return singleton
}

// Stop stops all loops and publishes a best-effort leave.
func (d *Discovery) Stop(ctx context.Context) error {
	defer d.releaseSingleton()
	if d.ctx == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var err error

	// Close consumer to unblock subscribe loops and then wait for goroutines.
	if d.consumer != nil {
		err = errors.Join(err, d.consumer.Close())
		d.consumer = nil
	}

	// Stop loops.
	d.cancel()
	// Wait for remaining goroutines.
	d.wg.Wait()

	// Best-effort leave publish.
	leaveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	err = errors.Join(err, d.publish(leaveCtx, EventLeave))
	cancel()

	// Close producer after leave has been published and loops have stopped.
	if d.producer != nil {
		err = errors.Join(err, d.producer.Close())
		d.producer = nil
	}

	d.ctx = nil
	d.cancel = nil
	return err
}

func (d *Discovery) publisherLoop(ctx context.Context) {
	// Publish join once.
	d.publish(ctx, EventJoin)

	t := time.NewTicker(d.dcfg.KeepAliveInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.publish(ctx, EventKeepAlive)
		}
	}
}

func (d *Discovery) sweeperLoop(ctx context.Context) {
	t := time.NewTicker(d.dcfg.SweepInterval)
	defer t.Stop()

	expiry := time.Duration(d.dcfg.ExpiryMultiplier) * d.dcfg.KeepAliveInterval

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.store.sweepExpired(time.Now().UTC(), expiry)
		}
	}
}

func (d *Discovery) publish(ctx context.Context, event EventType) error {
	msg := WireMessage{
		Event:           event,
		ID:              d.self.ID,
		Key:             d.self.Key,
		PrivateEndPoint: d.self.PrivateEndPoint,
		PublicEndPoint:  d.self.PublicEndPoint,
		Type:            d.self.Type,
		Version:         d.self.Version,
	}
	// Kafka message key for partitioning and per-instance ordering.
	return d.producer.PublishJSON(ctx, d.dcfg.Topic, d.self.PrivateEndPoint, msg)
}

func (d *Discovery) handleMessage(ctx context.Context, m *kafka.Message) error {
	if m == nil {
		return nil
	}
	var wm WireMessage
	if err := json.Unmarshal(m.Value, &wm); err != nil {
		// Ignore invalid JSON.
		return nil
	}
	// Validate minimal fields.
	if wm.Type == "" || wm.PrivateEndPoint == "" || wm.ID == 0 {
		return nil
	}
	// Never store self.
	if wm.PrivateEndPoint == d.self.PrivateEndPoint || wm.ID == d.self.ID {
		return nil
	}

	now := time.Now().UTC()
	switch wm.Event {
	case EventJoin, EventKeepAlive:
		d.store.upsert(Instance{
			ID:              wm.ID,
			Key:             wm.Key,
			Type:            wm.Type,
			Version:         wm.Version,
			PrivateEndPoint: wm.PrivateEndPoint,
			PublicEndPoint:  wm.PublicEndPoint,
			LastSeenUTC:     now,
		})
	case EventLeave:
		d.store.removeByTypeID(wm.Type, wm.ID)
	default:
		return nil
	}
	return nil
}

func mustRandInt64() int64 {
	max := new(big.Int).SetUint64(^uint64(0) >> 1) // MaxInt64

	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		panic(err)
	}
	return n.Int64() + 1
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// MarshalWireMessage is exposed for tests/diagnostics.
func MarshalWireMessage(m WireMessage) ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalWireMessage is exposed for tests/diagnostics.
func UnmarshalWireMessage(b []byte) (WireMessage, error) {
	var m WireMessage
	err := json.Unmarshal(b, &m)
	return m, err
}

var ErrNotStarted = errors.New("discovery not started")

// PublishNow publishes a discovery event immediately. Useful for diagnostics.
func (d *Discovery) PublishNow(ctx context.Context, event EventType) error {
	if d.producer == nil {
		return ErrNotStarted
	}
	switch event {
	case EventJoin, EventKeepAlive, EventLeave:
		return d.publish(ctx, event)
	default:
		return fmt.Errorf("unknown event: %s", event)
	}
}

// SetOrdering updates the store ordering strategy.
func (d *Discovery) SetOrdering(ordering OrderingStrategy) {
	d.store.setOrdering(ordering)
}

func (d *Discovery) releaseSingleton() {
	singletonMu.Lock()
	defer singletonMu.Unlock()
	if singleton == d {
		singleton = nil
	}
}
