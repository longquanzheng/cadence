// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package worker

import (
	"sync/atomic"
	"time"

	"github.com/uber/cadence/.gen/go/shared"
	"github.com/uber/cadence/common"
	carchiver "github.com/uber/cadence/common/archiver"
	"github.com/uber/cadence/common/cache"
	"github.com/uber/cadence/common/definition"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/loggerimpl"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/persistence"
	persistencefactory "github.com/uber/cadence/common/persistence/persistence-factory"
	"github.com/uber/cadence/common/service"
	"github.com/uber/cadence/common/service/dynamicconfig"
	"github.com/uber/cadence/service/worker/archiver"
	"github.com/uber/cadence/service/worker/batcher"
	"github.com/uber/cadence/service/worker/indexer"
	"github.com/uber/cadence/service/worker/replicator"
	"github.com/uber/cadence/service/worker/scanner"
)

type (
	// Service represents the cadence-worker service. This service hosts all background processing needed for cadence cluster:
	// 1. Replicator: Handles applying replication tasks generated by remote clusters.
	// 2. Indexer: Handles uploading of visibility records to elastic search.
	// 3. Archiver: Handles archival of workflow histories.
	Service struct {
		stopC         chan struct{}
		isStopped     int32
		params        *service.BootstrapParams
		config        *Config
		logger        log.Logger
		metricsClient metrics.Client
	}

	// Config contains all the service config for worker
	Config struct {
		ReplicationCfg  *replicator.Config
		ArchiverConfig  *archiver.Config
		IndexerCfg      *indexer.Config
		ScannerCfg      *scanner.Config
		BatcherCfg      *batcher.Config
		ThrottledLogRPS dynamicconfig.IntPropertyFn
		EnableBatcher   dynamicconfig.BoolPropertyFn
	}
)

const domainRefreshInterval = time.Second * 30

// NewService builds a new cadence-worker service
func NewService(params *service.BootstrapParams) common.Daemon {
	config := NewConfig(params)
	params.ThrottledLogger = loggerimpl.NewThrottledLogger(params.Logger, config.ThrottledLogRPS)
	params.UpdateLoggerWithServiceName(common.WorkerServiceName)
	return &Service{
		params: params,
		config: config,
		stopC:  make(chan struct{}),
	}
}

// NewConfig builds the new Config for cadence-worker service
func NewConfig(params *service.BootstrapParams) *Config {
	dc := dynamicconfig.NewCollection(params.DynamicConfig, params.Logger)
	config := &Config{
		ReplicationCfg: &replicator.Config{
			PersistenceMaxQPS:                  dc.GetIntProperty(dynamicconfig.WorkerPersistenceMaxQPS, 500),
			ReplicatorMetaTaskConcurrency:      dc.GetIntProperty(dynamicconfig.WorkerReplicatorMetaTaskConcurrency, 64),
			ReplicatorTaskConcurrency:          dc.GetIntProperty(dynamicconfig.WorkerReplicatorTaskConcurrency, 256),
			ReplicatorMessageConcurrency:       dc.GetIntProperty(dynamicconfig.WorkerReplicatorMessageConcurrency, 2048),
			ReplicatorActivityBufferRetryCount: dc.GetIntProperty(dynamicconfig.WorkerReplicatorActivityBufferRetryCount, 8),
			ReplicatorHistoryBufferRetryCount:  dc.GetIntProperty(dynamicconfig.WorkerReplicatorHistoryBufferRetryCount, 8),
			ReplicationTaskMaxRetryCount:       dc.GetIntProperty(dynamicconfig.WorkerReplicationTaskMaxRetryCount, 400),
			ReplicationTaskMaxRetryDuration:    dc.GetDurationProperty(dynamicconfig.WorkerReplicationTaskMaxRetryDuration, 15*time.Minute),
		},
		ArchiverConfig: &archiver.Config{
			ArchiverConcurrency:           dc.GetIntProperty(dynamicconfig.WorkerArchiverConcurrency, 50),
			ArchivalsPerIteration:         dc.GetIntProperty(dynamicconfig.WorkerArchivalsPerIteration, 1000),
			TimeLimitPerArchivalIteration: dc.GetDurationProperty(dynamicconfig.WorkerTimeLimitPerArchivalIteration, archiver.MaxArchivalIterationTimeout()),
		},
		ScannerCfg: &scanner.Config{
			PersistenceMaxQPS: dc.GetIntProperty(dynamicconfig.ScannerPersistenceMaxQPS, 100),
			Persistence:       &params.PersistenceConfig,
			ClusterMetadata:   params.ClusterMetadata,
		},
		BatcherCfg: &batcher.Config{
			AdminOperationToken: dc.GetStringProperty(dynamicconfig.AdminOperationToken, common.DefaultAdminOperationToken),
			ClusterMetadata:     params.ClusterMetadata,
		},
		EnableBatcher:   dc.GetBoolProperty(dynamicconfig.EnableBatcher, false),
		ThrottledLogRPS: dc.GetIntProperty(dynamicconfig.WorkerThrottledLogRPS, 20),
	}
	advancedVisWritingMode := dc.GetStringProperty(
		dynamicconfig.AdvancedVisibilityWritingMode,
		common.GetDefaultAdvancedVisibilityWritingMode(params.PersistenceConfig.IsAdvancedVisibilityConfigExist()),
	)
	if advancedVisWritingMode() != common.AdvancedVisibilityWritingModeOff {
		config.IndexerCfg = &indexer.Config{
			IndexerConcurrency:       dc.GetIntProperty(dynamicconfig.WorkerIndexerConcurrency, 1000),
			ESProcessorNumOfWorkers:  dc.GetIntProperty(dynamicconfig.WorkerESProcessorNumOfWorkers, 1),
			ESProcessorBulkActions:   dc.GetIntProperty(dynamicconfig.WorkerESProcessorBulkActions, 1000),
			ESProcessorBulkSize:      dc.GetIntProperty(dynamicconfig.WorkerESProcessorBulkSize, 2<<24), // 16MB
			ESProcessorFlushInterval: dc.GetDurationProperty(dynamicconfig.WorkerESProcessorFlushInterval, 1*time.Second),
			ValidSearchAttributes:    dc.GetMapProperty(dynamicconfig.ValidSearchAttributes, definition.GetDefaultIndexedKeys()),
		}
	}
	return config
}

// Start is called to start the service
func (s *Service) Start() {
	base := service.New(s.params)
	base.Start()
	s.logger = base.GetLogger()
	s.metricsClient = base.GetMetricsClient()
	s.logger.Info("service starting", tag.ComponentWorker)

	if s.config.IndexerCfg != nil {
		s.startIndexer(base)
	}

	replicatorEnabled := base.GetClusterMetadata().IsGlobalDomainEnabled()
	archiverEnabled := base.GetArchivalMetadata().GetHistoryConfig().ClusterConfiguredForArchival()
	batcherEnabled := s.config.EnableBatcher()

	pConfig := s.params.PersistenceConfig
	pConfig.SetMaxQPS(pConfig.DefaultStore, s.config.ReplicationCfg.PersistenceMaxQPS())
	pFactory := persistencefactory.New(&pConfig, s.params.ClusterMetadata.GetCurrentClusterName(), s.metricsClient, s.logger)
	s.ensureSystemDomainExists(pFactory, base.GetClusterMetadata().GetCurrentClusterName())

	s.startScanner(base)
	if replicatorEnabled {
		s.startReplicator(base, pFactory)
	}
	if archiverEnabled {
		s.startArchiver(base, pFactory)
	}
	if batcherEnabled {
		s.startBatcher(base)
	}

	s.logger.Info("service started", tag.ComponentWorker)
	<-s.stopC
	base.Stop()
}

// Stop is called to stop the service
func (s *Service) Stop() {
	if !atomic.CompareAndSwapInt32(&s.isStopped, 0, 1) {
		return
	}
	close(s.stopC)
	s.params.Logger.Info("service stopped", tag.ComponentWorker)
}

func (s *Service) startBatcher(base service.Service) {
	params := &batcher.BootstrapParams{
		Config:        *s.config.BatcherCfg,
		ServiceClient: s.params.PublicClient,
		MetricsClient: s.metricsClient,
		Logger:        s.logger,
		TallyScope:    s.params.MetricScope,
		ClientBean:    base.GetClientBean(),
	}
	batcher := batcher.New(params)
	if err := batcher.Start(); err != nil {
		s.logger.Fatal("error starting batcher", tag.Error(err))
	}
}

func (s *Service) startScanner(base service.Service) {
	params := &scanner.BootstrapParams{
		Config:        *s.config.ScannerCfg,
		SDKClient:     s.params.PublicClient,
		MetricsClient: s.metricsClient,
		Logger:        s.logger,
		TallyScope:    s.params.MetricScope,
	}
	scanner := scanner.New(params)
	if err := scanner.Start(); err != nil {
		s.logger.Fatal("error starting scanner", tag.Error(err))
	}
}

func (s *Service) startReplicator(base service.Service, pFactory persistencefactory.Factory) {
	metadataV2Mgr, err := pFactory.NewMetadataManager(persistencefactory.MetadataV2)
	if err != nil {
		s.logger.Fatal("failed to start replicator, could not create MetadataManager", tag.Error(err))
	}
	domainCache := cache.NewDomainCache(metadataV2Mgr, base.GetClusterMetadata(), s.metricsClient, s.logger)
	domainCache.Start()

	replicator := replicator.NewReplicator(
		base.GetClusterMetadata(),
		metadataV2Mgr,
		domainCache,
		base.GetClientBean(),
		s.config.ReplicationCfg,
		base.GetMessagingClient(),
		s.logger,
		s.metricsClient)
	if err := replicator.Start(); err != nil {
		replicator.Stop()
		s.logger.Fatal("fail to start replicator", tag.Error(err))
	}
}

func (s *Service) startIndexer(base service.Service) {
	indexer := indexer.NewIndexer(
		s.config.IndexerCfg,
		base.GetMessagingClient(),
		s.params.ESClient,
		s.params.ESConfig,
		s.logger,
		s.metricsClient)
	if err := indexer.Start(); err != nil {
		indexer.Stop()
		s.logger.Fatal("fail to start indexer", tag.Error(err))
	}
}

func (s *Service) startArchiver(base service.Service, pFactory persistencefactory.Factory) {
	publicClient := s.params.PublicClient

	historyManager, err := pFactory.NewHistoryManager()
	if err != nil {
		s.logger.Fatal("failed to start archiver, could not create HistoryManager", tag.Error(err))
	}
	historyV2Manager, err := pFactory.NewHistoryV2Manager()
	if err != nil {
		s.logger.Fatal("failed to start archiver, could not create HistoryV2Manager", tag.Error(err))
	}
	metadataMgr, err := pFactory.NewMetadataManager(persistencefactory.MetadataV1V2)
	if err != nil {
		s.logger.Fatal("failed to start archiver, could not create MetadataManager", tag.Error(err))
	}
	domainCache := cache.NewDomainCache(metadataMgr, s.params.ClusterMetadata, s.metricsClient, s.logger)
	domainCache.Start()
	historyArchiverBootstrapContainer := &carchiver.HistoryBootstrapContainer{
		HistoryManager:   historyManager,
		HistoryV2Manager: historyV2Manager,
		Logger:           s.logger,
		MetricsClient:    s.metricsClient,
		ClusterMetadata:  base.GetClusterMetadata(),
		DomainCache:      domainCache,
	}
	archiverProvider := base.GetArchiverProvider()
	err = archiverProvider.RegisterBootstrapContainer(common.WorkerServiceName, historyArchiverBootstrapContainer, &carchiver.VisibilityBootstrapContainer{})
	if err != nil {
		s.logger.Fatal("failed to register archiver bootstrap container", tag.Error(err))
	}

	bc := &archiver.BootstrapContainer{
		PublicClient:     publicClient,
		MetricsClient:    s.metricsClient,
		Logger:           s.logger,
		HistoryManager:   historyManager,
		HistoryV2Manager: historyV2Manager,
		DomainCache:      domainCache,
		Config:           s.config.ArchiverConfig,
		ArchiverProvider: archiverProvider,
	}
	clientWorker := archiver.NewClientWorker(bc)
	if err := clientWorker.Start(); err != nil {
		clientWorker.Stop()
		s.logger.Fatal("failed to start archiver", tag.Error(err))
	}
}

func (s *Service) ensureSystemDomainExists(pFactory persistencefactory.Factory, clusterName string) {
	metadataProxy, err := pFactory.NewMetadataManager(persistencefactory.MetadataV1V2)
	if err != nil {
		s.logger.Fatal("error creating metadataMgr proxy", tag.Error(err))
	}
	defer metadataProxy.Close()
	_, err = metadataProxy.GetDomain(&persistence.GetDomainRequest{Name: common.SystemLocalDomainName})
	switch err.(type) {
	case nil:
		return
	case *shared.EntityNotExistsError:
		s.logger.Info("cadence-system domain does not exist, attempting to register domain")
		s.registerSystemDomain(pFactory, clusterName)
	default:
		s.logger.Fatal("failed to verify if cadence system domain exists", tag.Error(err))
	}
}

func (s *Service) registerSystemDomain(pFactory persistencefactory.Factory, clusterName string) {
	metadataV2, err := pFactory.NewMetadataManager(persistencefactory.MetadataV2)
	if err != nil {
		s.logger.Fatal("error creating metadataV2Mgr", tag.Error(err))
	}
	defer metadataV2.Close()
	_, err = metadataV2.CreateDomain(&persistence.CreateDomainRequest{
		Info: &persistence.DomainInfo{
			ID:          common.SystemDomainID,
			Name:        common.SystemLocalDomainName,
			Description: "Cadence internal system domain",
		},
		Config: &persistence.DomainConfig{
			Retention:  common.SystemDomainRetentionDays,
			EmitMetric: true,
		},
		ReplicationConfig: &persistence.DomainReplicationConfig{
			ActiveClusterName: clusterName,
			Clusters:          persistence.GetOrUseDefaultClusters(clusterName, nil),
		},
		IsGlobalDomain:  false,
		FailoverVersion: common.EmptyVersion,
	})
	if err != nil {
		if _, ok := err.(*shared.DomainAlreadyExistsError); ok {
			return
		}
		s.logger.Fatal("failed to register system domain", tag.Error(err))
	}
	// this is needed because frontend domainCache will take about 10s to load the
	// domain after its created first time. Archiver/Scanner cannot start their cadence
	// workers until this refresh happens
	time.Sleep(domainRefreshInterval)
}
