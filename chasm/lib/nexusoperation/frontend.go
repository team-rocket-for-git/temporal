package nexusoperation

import (
	"context"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	nexusoperationpb "go.temporal.io/server/chasm/lib/nexusoperation/gen/nexusoperationpb/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/namespace"
)

// FrontendHandler provides the frontend-facing API for standalone Nexus operations.
type FrontendHandler interface {
	StartNexusOperationExecution(context.Context, *workflowservice.StartNexusOperationExecutionRequest) (*workflowservice.StartNexusOperationExecutionResponse, error)
	DescribeNexusOperationExecution(context.Context, *workflowservice.DescribeNexusOperationExecutionRequest) (*workflowservice.DescribeNexusOperationExecutionResponse, error)
	PollNexusOperationExecution(context.Context, *workflowservice.PollNexusOperationExecutionRequest) (*workflowservice.PollNexusOperationExecutionResponse, error)
	ListNexusOperationExecutions(context.Context, *workflowservice.ListNexusOperationExecutionsRequest) (*workflowservice.ListNexusOperationExecutionsResponse, error)
	CountNexusOperationExecutions(context.Context, *workflowservice.CountNexusOperationExecutionsRequest) (*workflowservice.CountNexusOperationExecutionsResponse, error)
	RequestCancelNexusOperationExecution(context.Context, *workflowservice.RequestCancelNexusOperationExecutionRequest) (*workflowservice.RequestCancelNexusOperationExecutionResponse, error)
	TerminateNexusOperationExecution(context.Context, *workflowservice.TerminateNexusOperationExecutionRequest) (*workflowservice.TerminateNexusOperationExecutionResponse, error)
	DeleteNexusOperationExecution(context.Context, *workflowservice.DeleteNexusOperationExecutionRequest) (*workflowservice.DeleteNexusOperationExecutionResponse, error)
	IsStandaloneNexusOperationEnabled(namespaceName string) bool
}

type frontendHandler struct {
	client            nexusoperationpb.NexusOperationServiceClient
	config            *Config
	logger            log.Logger
	namespaceRegistry namespace.Registry
}

func NewFrontendHandler(
	client nexusoperationpb.NexusOperationServiceClient,
	config *Config,
	logger log.Logger,
	namespaceRegistry namespace.Registry,
) FrontendHandler {
	return &frontendHandler{
		client:            client,
		config:            config,
		logger:            logger,
		namespaceRegistry: namespaceRegistry,
	}
}

func (h *frontendHandler) IsStandaloneNexusOperationEnabled(namespaceName string) bool {
	return h.config.Enabled() && h.config.ChasmEnabled(namespaceName) && h.config.ChasmNexusEnabled(namespaceName)
}

// StartNexusOperationExecution initiates a standalone Nexus operation execution in the specified namespace.
// It validates the request, resolves the namespace ID, and forwards the request to the Nexus operation service handler.
func (h *frontendHandler) StartNexusOperationExecution(
	ctx context.Context,
	req *workflowservice.StartNexusOperationExecutionRequest,
) (*workflowservice.StartNexusOperationExecutionResponse, error) {
	if !h.config.Enabled() {
		return nil, serviceerror.NewUnimplemented("Nexus operations disabled")
	}
	if !h.config.ChasmEnabled(req.GetNamespace()) || !h.config.ChasmNexusEnabled(req.GetNamespace()) {
		return nil, serviceerror.NewUnimplemented("Standalone Nexus operation is disabled")
	}

	if err := validateStartNexusOperationExecutionRequest(req, h.config); err != nil {
		return nil, err
	}

	namespaceID, err := h.namespaceRegistry.GetNamespaceID(namespace.Name(req.GetNamespace()))
	if err != nil {
		return nil, err
	}

	resp, err := h.client.StartNexusOperation(ctx, &nexusoperationpb.StartNexusOperationRequest{
		NamespaceId:     namespaceID.String(),
		FrontendRequest: req,
	})

	return resp.GetFrontendResponse(), err
}

// DescribeNexusOperationExecution queries current operation state.
func (h *frontendHandler) DescribeNexusOperationExecution(
	ctx context.Context,
	req *workflowservice.DescribeNexusOperationExecutionRequest,
) (*workflowservice.DescribeNexusOperationExecutionResponse, error) {
	if !h.config.Enabled() {
		return nil, serviceerror.NewUnimplemented("Nexus operations disabled")
	}
	if !h.config.ChasmEnabled(req.GetNamespace()) || !h.config.ChasmNexusEnabled(req.GetNamespace()) {
		return nil, serviceerror.NewUnimplemented("Standalone Nexus operation is disabled")
	}

	if err := validateDescribeNexusOperationExecutionRequest(req, h.config); err != nil {
		return nil, err
	}

	namespaceID, err := h.namespaceRegistry.GetNamespaceID(namespace.Name(req.GetNamespace()))
	if err != nil {
		return nil, err
	}

	resp, err := h.client.DescribeNexusOperation(ctx, &nexusoperationpb.DescribeNexusOperationRequest{
		NamespaceId:     namespaceID.String(),
		FrontendRequest: req,
	})
	return resp.GetFrontendResponse(), err
}

func (h *frontendHandler) PollNexusOperationExecution(context.Context, *workflowservice.PollNexusOperationExecutionRequest) (*workflowservice.PollNexusOperationExecutionResponse, error) {
	return nil, serviceerror.NewUnimplemented("PollNexusOperationExecution not implemented")
}

func (h *frontendHandler) ListNexusOperationExecutions(context.Context, *workflowservice.ListNexusOperationExecutionsRequest) (*workflowservice.ListNexusOperationExecutionsResponse, error) {
	return nil, serviceerror.NewUnimplemented("ListNexusOperationExecutions not implemented")
}

func (h *frontendHandler) CountNexusOperationExecutions(context.Context, *workflowservice.CountNexusOperationExecutionsRequest) (*workflowservice.CountNexusOperationExecutionsResponse, error) {
	return nil, serviceerror.NewUnimplemented("CountNexusOperationExecutions not implemented")
}

func (h *frontendHandler) RequestCancelNexusOperationExecution(context.Context, *workflowservice.RequestCancelNexusOperationExecutionRequest) (*workflowservice.RequestCancelNexusOperationExecutionResponse, error) {
	return nil, serviceerror.NewUnimplemented("RequestCancelNexusOperationExecution not implemented")
}

func (h *frontendHandler) TerminateNexusOperationExecution(context.Context, *workflowservice.TerminateNexusOperationExecutionRequest) (*workflowservice.TerminateNexusOperationExecutionResponse, error) {
	return nil, serviceerror.NewUnimplemented("TerminateNexusOperationExecution not implemented")
}

func (h *frontendHandler) DeleteNexusOperationExecution(context.Context, *workflowservice.DeleteNexusOperationExecutionRequest) (*workflowservice.DeleteNexusOperationExecutionResponse, error) {
	return nil, serviceerror.NewUnimplemented("DeleteNexusOperationExecution not implemented")
}

func validateStartNexusOperationExecutionRequest(req *workflowservice.StartNexusOperationExecutionRequest, config *Config) error {
	ns := req.GetNamespace()
	if ns == "" {
		return serviceerror.NewInvalidArgument("namespace is required")
	}
	maxIDLen := config.MaxIDLengthLimit()
	if req.GetOperationId() == "" {
		return serviceerror.NewInvalidArgument("operation_id is required")
	}
	if len(req.GetOperationId()) > maxIDLen {
		return serviceerror.NewInvalidArgumentf("operation_id exceeds length limit. Length=%d Limit=%d",
			len(req.GetOperationId()), maxIDLen)
	}
	if req.GetEndpoint() == "" {
		return serviceerror.NewInvalidArgument("endpoint is required")
	}
	if req.GetService() == "" {
		return serviceerror.NewInvalidArgument("service is required")
	}
	if len(req.GetService()) > config.MaxServiceNameLength(ns) {
		return serviceerror.NewInvalidArgumentf("service name exceeds length limit. Length=%d Limit=%d",
			len(req.GetService()), config.MaxServiceNameLength(ns))
	}
	if req.GetOperation() == "" {
		return serviceerror.NewInvalidArgument("operation is required")
	}
	if len(req.GetOperation()) > config.MaxOperationNameLength(ns) {
		return serviceerror.NewInvalidArgumentf("operation name exceeds length limit. Length=%d Limit=%d",
			len(req.GetOperation()), config.MaxOperationNameLength(ns))
	}
	return nil
}

func validateDescribeNexusOperationExecutionRequest(req *workflowservice.DescribeNexusOperationExecutionRequest, config *Config) error {
	if req.GetNamespace() == "" {
		return serviceerror.NewInvalidArgument("namespace is required")
	}
	maxIDLen := config.MaxIDLengthLimit()
	if req.GetOperationId() == "" {
		return serviceerror.NewInvalidArgument("operation_id is required")
	}
	if len(req.GetOperationId()) > maxIDLen {
		return serviceerror.NewInvalidArgumentf("operation_id exceeds length limit. Length=%d Limit=%d",
			len(req.GetOperationId()), maxIDLen)
	}
	if len(req.GetRunId()) > maxIDLen {
		return serviceerror.NewInvalidArgumentf("run_id exceeds length limit. Length=%d Limit=%d",
			len(req.GetRunId()), maxIDLen)
	}
	// TODO: Add long-poll validation (run_id required when long_poll_token is set).
	return nil
}
