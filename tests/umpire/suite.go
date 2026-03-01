package catch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.temporal.io/api/workflowservice/v1"
	sdkclient "go.temporal.io/sdk/client"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/telemetry"
	catchpkg "go.temporal.io/server/common/testing/bats"
	"go.temporal.io/server/common/testing/testlogger"
	"go.temporal.io/server/tests/testcore"
	"go.temporal.io/server/tools/umpire"
	"go.temporal.io/server/tools/umpire/pitcher"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
)

// TestSuite provides test infrastructure for CATCH tests
type TestSuite struct {
	t           testing.TB
	logger      log.Logger
	testCluster *testcore.TestCluster
	namespace   namespace.Name
	sdkClient   sdkclient.Client
	pitcher     pitcher.Pitcher
	umpire      *umpire.Umpire
}

// NewTestSuite creates a new CATCH test suite
func NewTestSuite(t testing.TB) *TestSuite {
	return &TestSuite{
		t: t,
	}
}

// Setup initializes the test cluster and components
func (s *TestSuite) Setup() {
	t, ok := s.t.(*testing.T)
	if !ok {
		// If not *testing.T, try to extract it from testing.TB
		panic("TestSuite requires *testing.T")
	}

	// Enable OTEL debug mode to capture payload data in spans
	s.t.Setenv("TEMPORAL_OTEL_DEBUG", "true")

	// Create logger
	tl := testlogger.NewTestLogger(s.t, testlogger.FailOnExpectedErrorOnly)
	testlogger.DontFailOnError(tl)
	tl.Expect(testlogger.Error, ".*", tag.FailedAssertion)
	s.logger = tl

	// Initialize umpire first
	var err error
	s.umpire, err = umpire.New(umpire.Config{
		Logger: s.logger,
	})
	require.NoError(s.t, err)

	// Set global umpire so interceptor can access it
	umpire.Set(s.umpire)

	// Create test cluster with pitcher enabled and catch interceptor
	clusterConfig := &testcore.TestClusterConfig{
		HistoryConfig: testcore.HistoryConfig{
			NumHistoryShards: 4,
		},
		EnableMetricsCapture: true,
		// Add catch interceptor to record moves
		AdditionalInterceptors: []grpc.UnaryServerInterceptor{
			catchpkg.UnaryServerInterceptor(),
		},
		// Configure SpanExporter to send OTEL spans to umpire
		SpanExporters: map[telemetry.SpanExporterType]sdktrace.SpanExporter{
			telemetry.OtelTracesOtlpExporterType: catchpkg.NewSpanExporter(s.umpire),
		},
	}

	testClusterFactory := testcore.NewTestClusterFactory()
	s.testCluster, err = testClusterFactory.NewCluster(t, clusterConfig, s.logger)
	require.NoError(s.t, err)

	// Get pitcher from test cluster
	s.pitcher = s.testCluster.Pitcher()

	// Register namespace using FrontendClient
	s.namespace = namespace.Name(testcore.RandomizeStr("catch-test-ns"))
	_, err = s.testCluster.FrontendClient().RegisterNamespace(context.Background(), &workflowservice.RegisterNamespaceRequest{
		Namespace:                        string(s.namespace),
		WorkflowExecutionRetentionPeriod: durationpb.New(24 * time.Hour),
	})
	require.NoError(s.t, err)

	// Create SDK client
	s.sdkClient, err = sdkclient.Dial(sdkclient.Options{
		HostPort:  s.testCluster.Host().FrontendGRPCAddress(),
		Namespace: string(s.namespace),
		Logger:    log.NewSdkLogger(s.logger),
	})
	require.NoError(s.t, err)

	// Setup cleanup
	s.t.Cleanup(func() {
		s.Teardown()
	})
}

// Teardown cleans up test resources
func (s *TestSuite) Teardown() {
	// Clear global umpire
	umpire.Set(nil)

	// Reset pitcher
	if s.pitcher != nil {
		s.pitcher.Reset()
	}

	// Close SDK client
	if s.sdkClient != nil {
		s.sdkClient.Close()
	}

	// Close umpire
	if s.umpire != nil {
		s.umpire.Close()
	}

	// Tear down test cluster
	if s.testCluster != nil {
		s.testCluster.TearDownCluster()
	}
}

// Logger returns the test logger
func (s *TestSuite) Logger() log.Logger {
	return s.logger
}

// TestCluster returns the test cluster
func (s *TestSuite) TestCluster() *testcore.TestCluster {
	return s.testCluster
}

// Namespace returns the test namespace
func (s *TestSuite) Namespace() namespace.Name {
	return s.namespace
}

// SdkClient returns the SDK client
func (s *TestSuite) SdkClient() sdkclient.Client {
	return s.sdkClient
}

// Pitcher returns the pitcher for fault injection
func (s *TestSuite) Pitcher() pitcher.Pitcher {
	return s.pitcher
}

// Umpire returns the umpire for verification
func (s *TestSuite) Umpire() *umpire.Umpire {
	return s.umpire
}

// TaskQueue returns a random task queue name
func (s *TestSuite) TaskQueue() string {
	return testcore.RandomizeStr("catch-tq")
}

// Run executes a test scenario
func (s *TestSuite) Run(t *testing.T, name string, fn func(ctx context.Context, t *testing.T, s *TestSuite)) {
	t.Run(name, func(t *testing.T) {
		ctx := context.Background()
		fn(ctx, t, s)
	})
}
