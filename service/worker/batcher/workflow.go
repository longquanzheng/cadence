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

package batcher

import (
	"context"
	"fmt"
	"time"

	s "github.com/uber/cadence/.gen/go/shared"
	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/metrics"
	"go.uber.org/cadence"
	"go.uber.org/cadence/.gen/go/shared"
	"go.uber.org/cadence/activity"
	cclient "go.uber.org/cadence/client"
	"go.uber.org/cadence/workflow"
	"golang.org/x/time/rate"
)

const (
	batcherContextKey   = "batcherContext"
	BatcherTaskListName = "cadence-sys-batcher-tasklist"
	BatchWFTypeName     = "cadence-sys-batch-workflow"
	BatchActivityName   = "cadence-sys-batch-activity"

	infiniteDuration = 20 * 365 * 24 * time.Hour
	pageSize         = 1000

	// below are default values for BatchParams
	defaultRPS                                = 50
	defaultRPSPerConcurrency                  = 10
	defaultAttemptsOnRetryableError           = 50
	defaultActivityHeartBeatTimeout           = time.Minute
	DefaultWorkflowStartToCloseTimeoutSeconds = 60 * 60 * 6 // 6 hours
	DecisionStartToCloseTimeoutSeconds        = 10
)

const (
	//TODO use client side import after the IDL is in client
	// BatchTypeTerminate is batch type for terminating workflows
	BatchTypeTerminate = int(s.BatchOperationTypeTerminate)
	// BatchTypeCancel is the batch type for canceling workflows
	BatchTypeCancel = int(s.BatchOperationTypeRequestCancel)
	// BatchTypeSignal is batch type for signaling workflows
	BatchTypeSignal = int(s.BatchOperationTypeSignal)
)

type (
	// TerminateParams is the parameters for terminating workflow
	TerminateParams struct {
		// this indicates whether to terminate children workflow. Default to true.
		// TODO https://github.com/uber/cadence/issues/2159
		// Ideally default should be childPolicy of the workflow. But it's currently totally broken.
		TerminateChildren *bool
	}

	// CancelParams is the parameters for canceling workflow
	CancelParams struct {
		// this indicates whether to cancel children workflow. Default to true.
		// TODO https://github.com/uber/cadence/issues/2159
		// Ideally default should be childPolicy of the workflow. But it's currently totally broken.
		CancelChildren *bool
	}

	// SignalParams is the parameters for signaling workflow
	SignalParams struct {
		SignalName string
		Input      string
	}

	// BatchParams is the parameters for batch operation workflow
	BatchParams struct {
		// Target domain to execute batch operation
		DomainName string
		// To get the target workflows for processing
		Query string
		// Reason for the operation
		Reason string
		// Supporting: terminate,requestCancel,signal
		BatchType int

		// Below are all optional
		// TerminateParams is params only for BatchTypeTerminate
		TerminateParams TerminateParams
		// CancelParams is params only for BatchTypeCancel
		CancelParams CancelParams
		// SignalParams is params only for BatchTypeSignal
		SignalParams SignalParams
		// RPS of processing. Default to defaultRPS
		// TODO we will implement smarter way than this static rate limiter: https://github.com/uber/cadence/issues/2138
		RPS int
		// Number of goroutines running in parallel to process
		Concurrency int
		// Number of attempts for each workflow to process in case of retryable error before giving up
		AttemptsOnRetryableError int
		// timeout for activity heartbeat
		ActivityHeartBeatTimeout time.Duration
		// errors that will not retry which consumes AttemptsOnRetryableError. Default to empty
		NonRetryableErrors []string
		// internal conversion for NonRetryableErrors
		_nonRetryableErrors map[string]struct{}
	}

	// HeartBeatDetails is the struct for heartbeat details
	HeartBeatDetails struct {
		PageToken   []byte
		CurrentPage int
		// This is just an estimation for visibility
		TotalEstimate int64
		// Number of workflows processed successfully
		SuccessCount int
		// Number of workflows that give up due to errors.
		ErrorCount int
	}

	taskDetail struct {
		execution shared.WorkflowExecution
		attempts  int
		// passing along the current heartbeat details to make heartbeat within a task so that it won't timeout
		hbd HeartBeatDetails
	}
)

var (
	batchActivityRetryPolicy = cadence.RetryPolicy{
		InitialInterval:    10 * time.Second,
		BackoffCoefficient: 1.7,
		MaximumInterval:    5 * time.Minute,
		ExpirationInterval: infiniteDuration,
	}

	batchActivityOptions = workflow.ActivityOptions{
		ScheduleToStartTimeout: 5 * time.Minute,
		StartToCloseTimeout:    infiniteDuration,
		RetryPolicy:            &batchActivityRetryPolicy,
	}
)

func init() {
	workflow.RegisterWithOptions(BatchWorkflow, workflow.RegisterOptions{Name: BatchWFTypeName})
	activity.RegisterWithOptions(BatchActivity, activity.RegisterOptions{Name: BatchActivityName})
}

// BatchWorkflow is the workflow that runs a batch job of resetting workflows
func BatchWorkflow(ctx workflow.Context, batchParams BatchParams) (HeartBeatDetails, error) {
	batchParams = setDefaultParams(batchParams)
	err := validateParams(batchParams)
	if err != nil {
		return HeartBeatDetails{}, err
	}
	batchActivityOptions.HeartbeatTimeout = batchParams.ActivityHeartBeatTimeout
	opt := workflow.WithActivityOptions(ctx, batchActivityOptions)
	var result HeartBeatDetails
	err = workflow.ExecuteActivity(opt, BatchActivityName, batchParams).Get(ctx, &result)
	return result, err
}

func validateParams(params BatchParams) error {
	if params.Reason == "" ||
		params.DomainName == "" ||
		params.Query == "" {
		return fmt.Errorf("must provide required parameters: BatchType/Reason/DomainName/Query")
	}
	switch params.BatchType {
	case BatchTypeSignal:
		if params.SignalParams.SignalName == "" {
			return fmt.Errorf("must provide signal name")
		}
		return nil
	case BatchTypeCancel:
		fallthrough
	case BatchTypeTerminate:
		return nil
	default:
		return fmt.Errorf("not supported batch type: %v", params.BatchType)
	}
}

func setDefaultParams(params BatchParams) BatchParams {
	if params.RPS <= 0 {
		params.RPS = defaultRPS
	}
	if params.Concurrency <= 0 {
		params.Concurrency = params.RPS / defaultRPSPerConcurrency
	}
	if params.AttemptsOnRetryableError <= 0 {
		params.AttemptsOnRetryableError = defaultAttemptsOnRetryableError
	}
	if params.ActivityHeartBeatTimeout <= 0 {
		params.ActivityHeartBeatTimeout = defaultActivityHeartBeatTimeout
	}
	if len(params.NonRetryableErrors) > 0 {
		params._nonRetryableErrors = make(map[string]struct{}, len(params.NonRetryableErrors))
		for _, estr := range params.NonRetryableErrors {
			params._nonRetryableErrors[estr] = struct{}{}
		}
	}
	if params.TerminateParams.TerminateChildren == nil {
		params.TerminateParams.TerminateChildren = common.BoolPtr(true)
	}
	return params
}

// BatchActivity is activity for processing batch operation
func BatchActivity(ctx context.Context, batchParams BatchParams) (HeartBeatDetails, error) {
	batcher := ctx.Value(batcherContextKey).(*Batcher)
	client := cclient.NewClient(batcher.svcClient, batchParams.DomainName, &cclient.Options{})

	hbd := HeartBeatDetails{}
	startOver := true
	if activity.HasHeartbeatDetails(ctx) {
		if err := activity.GetHeartbeatDetails(ctx, &hbd); err == nil {
			startOver = false
		} else {
			batcher := ctx.Value(batcherContextKey).(*Batcher)
			batcher.metricsClient.IncCounter(metrics.BatcherScope, metrics.BatcherProcessorFailures)
			getActivityLogger(ctx).Error("Failed to recover from last heartbeat, start over from beginning", tag.Error(err))
		}
	}

	if startOver {
		resp, err := client.CountWorkflow(ctx, &shared.CountWorkflowExecutionsRequest{
			Query: common.StringPtr(batchParams.Query),
		})
		if err != nil {
			return HeartBeatDetails{}, err
		}
		hbd.TotalEstimate = resp.GetCount()
	}
	rateLimiter := rate.NewLimiter(rate.Limit(batchParams.RPS), batchParams.RPS)
	taskCh := make(chan taskDetail, pageSize)
	respCh := make(chan error, pageSize)
	for i := 0; i < batchParams.Concurrency; i++ {
		go startTaskProcessor(ctx, batchParams, taskCh, respCh, rateLimiter, client)
	}

	for {
		// TODO https://github.com/uber/cadence/issues/2154
		//  Need to improve scan concurrency because it will hold an ES resource until the workflow finishes.
		//  And we can't use list API because terminate / reset will mutate the result.
		resp, err := client.ScanWorkflow(ctx, &shared.ListWorkflowExecutionsRequest{
			PageSize:      common.Int32Ptr(int32(pageSize)),
			NextPageToken: hbd.PageToken,
			Query:         common.StringPtr(batchParams.Query),
		})
		if err != nil {
			return HeartBeatDetails{}, err
		}
		batchCount := len(resp.Executions)
		if batchCount <= 0 {
			break
		}

		// send all tasks
		for _, wf := range resp.Executions {
			taskCh <- taskDetail{
				execution: *wf.Execution,
				attempts:  0,
				hbd:       hbd,
			}
		}

		succCount := 0
		errCount := 0
		// wait for counters indicate this batch is done
	Loop:
		for {
			select {
			case err := <-respCh:
				if err == nil {
					succCount++
				} else {
					errCount++
				}
				if succCount+errCount == batchCount {
					break Loop
				}
			case <-ctx.Done():
				return HeartBeatDetails{}, ctx.Err()
			}
		}

		hbd.CurrentPage++
		hbd.PageToken = resp.NextPageToken
		hbd.SuccessCount += succCount
		hbd.ErrorCount += errCount
		activity.RecordHeartbeat(ctx, hbd)

		if len(hbd.PageToken) == 0 {
			break
		}
	}

	return hbd, nil
}

func startTaskProcessor(
	ctx context.Context,
	batchParams BatchParams,
	taskCh chan taskDetail,
	respCh chan error,
	limiter *rate.Limiter,
	client cclient.Client,
) {
	batcher := ctx.Value(batcherContextKey).(*Batcher)
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-taskCh:
			if isDone(ctx) {
				return
			}
			var err error

			switch batchParams.BatchType {
			case BatchTypeTerminate:
				err = processTask(ctx, limiter, task, batchParams, client,
					batchParams.TerminateParams.TerminateChildren,
					func(workflowID, runID string) error {
						return client.TerminateWorkflow(ctx, workflowID, runID, batchParams.Reason, []byte{})
					})
			case BatchTypeCancel:
				err = processTask(ctx, limiter, task, batchParams, client,
					batchParams.CancelParams.CancelChildren,
					func(workflowID, runID string) error {
						return client.CancelWorkflow(ctx, workflowID, runID)
					})
			case BatchTypeSignal:
				err = processTask(ctx, limiter, task, batchParams, client, common.BoolPtr(false),
					func(workflowID, runID string) error {
						return client.SignalWorkflow(ctx, workflowID, runID,
							batchParams.SignalParams.SignalName, []byte(batchParams.SignalParams.Input))
					})
			}
			if err != nil {
				batcher.metricsClient.IncCounter(metrics.BatcherScope, metrics.BatcherProcessorFailures)
				getActivityLogger(ctx).Error("Failed to process batch operation task", tag.Error(err))

				_, ok := batchParams._nonRetryableErrors[err.Error()]
				if ok || task.attempts >= batchParams.AttemptsOnRetryableError {
					respCh <- err
				} else {
					// put back to the channel if less than attemptsOnError
					task.attempts++
					taskCh <- task
				}
			} else {
				batcher.metricsClient.IncCounter(metrics.BatcherScope, metrics.BatcherProcessorSuccess)
				respCh <- nil
			}
		}
	}
}

func processTask(
	ctx context.Context,
	limiter *rate.Limiter,
	task taskDetail,
	batchParams BatchParams,
	client cclient.Client,
	applyOnChild *bool,
	procFn func(string, string) error,
) error {
	wfs := []shared.WorkflowExecution{task.execution}
	for len(wfs) > 0 {
		wf := wfs[0]

		err := limiter.Wait(ctx)
		if err != nil {
			return err
		}

		err = procFn(wf.GetWorkflowId(), wf.GetRunId())
		if err != nil {
			// EntityNotExistsError means wf is not running or deleted
			_, ok := err.(*shared.EntityNotExistsError)
			if !ok {
				return err
			}
		}
		wfs = wfs[1:]
		resp, err := client.DescribeWorkflowExecution(ctx, wf.GetWorkflowId(), wf.GetRunId())
		if err != nil {
			// EntityNotExistsError means wf is deleted
			_, ok := err.(*shared.EntityNotExistsError)
			if !ok {
				return err
			}
			continue
		}

		// TODO https://github.com/uber/cadence/issues/2159
		// By default should use ChildPolicy, but it is totally broken in Cadence, we need to fix it before using
		if applyOnChild != nil && *applyOnChild && len(resp.PendingChildren) > 0 {
			getActivityLogger(ctx).Info("Found more child workflows to process", tag.Number(int64(len(resp.PendingChildren))))
			for _, ch := range resp.PendingChildren {
				wfs = append(wfs, shared.WorkflowExecution{
					WorkflowId: ch.WorkflowID,
					RunId:      ch.RunID,
				})
			}
		}

		activity.RecordHeartbeat(ctx, task.hbd)
	}

	return nil
}

func isDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func getActivityLogger(ctx context.Context) log.Logger {
	batcher := ctx.Value(batcherContextKey).(*Batcher)
	wfInfo := activity.GetInfo(ctx)
	return batcher.logger.WithTags(
		tag.WorkflowID(wfInfo.WorkflowExecution.ID),
		tag.WorkflowRunID(wfInfo.WorkflowExecution.RunID),
		tag.WorkflowDomainName(wfInfo.WorkflowDomain),
	)
}
