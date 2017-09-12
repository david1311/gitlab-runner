package commands

import (
	"context"
	"io/ioutil"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"gitlab.com/gitlab-org/gitlab-runner/common"
)

func init() {
	s := common.MockShell{}
	s.On("GetName").Return("script-shell")
	s.On("GenerateScript", mock.Anything, mock.Anything).Return("script", nil)
	common.RegisterShell(&s)
}

type jobSimulation func(mock.Arguments)

func TestSingleRunnerSigquit(t *testing.T) {
	job := func(_ mock.Arguments) {
		err := syscall.Kill(syscall.Getpid(), syscall.SIGQUIT)
		assert.NoError(t, err)
		// simulate some real work while while sigquit get handled
		time.Sleep(time.Second)
	}

	single, cleanup := mockingExecutionStack(t, "test-sigquit", 1, job)
	defer cleanup()

	single.Execute(nil)
}

func TestSingleRunnerMaxBuilds(t *testing.T) {
	maxBuilds := 7

	single, cleanup := mockingExecutionStack(t, "test-max-build", maxBuilds, nil)
	defer cleanup()

	single.Execute(nil)
}

func newRunSingleCommand(executorName string, network common.Network) *RunSingleCommand {
	return &RunSingleCommand{
		network: network,
		RunnerConfig: common.RunnerConfig{
			RunnerSettings: common.RunnerSettings{
				Executor: executorName,
			},
			RunnerCredentials: common.RunnerCredentials{
				URL:   "http://example.com",
				Token: "_test_token_",
			},
		},
	}
}

func mockingExecutionStack(t *testing.T, executorName string, maxBuilds int, job jobSimulation) (*RunSingleCommand, func()) {
	// mocking the whole stack
	e := common.MockExecutor{}
	p := common.MockExecutorProvider{}
	mockNetwork := common.MockNetwork{}

	//Network
	jobData := common.JobResponse{}
	_, cancel := context.WithCancel(context.Background())
	jobTrace := common.Trace{Writer: ioutil.Discard, CancelFunc: cancel}
	mockNetwork.On("RequestJob", mock.Anything).Return(&jobData, true).Times(maxBuilds)
	processJob := mockNetwork.On("ProcessJob", mock.Anything, mock.Anything).Return(&jobTrace).Times(maxBuilds)
	if job != nil {
		processJob.Run(job)
	}

	//ExecutorProvider
	p.On("Create").Return(&e).Times(maxBuilds)
	p.On("Acquire", mock.Anything).Return(&common.MockExecutorData{}, nil).Times(maxBuilds)
	p.On("Release", mock.Anything, mock.Anything).Return(nil).Times(maxBuilds)

	//Executor
	e.On("Prepare", mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(maxBuilds)
	e.On("Finish", nil).Return().Times(maxBuilds)
	e.On("Cleanup").Return().Times(maxBuilds)

	// Run script successfully
	e.On("Shell").Return(&common.ShellScriptInfo{Shell: "script-shell"})
	e.On("Run", mock.Anything).Return(nil)

	common.RegisterExecutor(executorName, &p)

	single := newRunSingleCommand(executorName, &mockNetwork)
	single.MaxBuilds = maxBuilds
	cleanup := func() {
		e.AssertExpectations(t)
		p.AssertExpectations(t)
		mockNetwork.AssertExpectations(t)
		cancel()
	}

	return single, cleanup
}