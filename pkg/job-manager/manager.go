package job_manager

import (
	"errors"
	"fmt"
	"github.com/kaytu-io/kaytu-util/pkg/concurrency"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"reflect"
	"time"
)

type ScheduledJobManager struct {
	logger *zap.Logger

	db *gorm.DB

	jobModel SchedulableJob
	MaxRetry int // Max number of times to retry a job before marking it as permanently failed

	MaxInFlightJobs   int           // Max number of jobs that can be queued + in progress at a time
	QueuedTimeout     time.Duration // How long to wait before a queued job is considered timed out
	InProgressTimeout time.Duration // How long to wait before an in progress job is considered timed out
	OldJobRetention   time.Duration // How long to keep old jobs around before deleting them

	EnqueueCheckInterval time.Duration // How often to check for new jobs to enqueue
	TimeoutCheckInterval time.Duration // How often to check for jobs that have timed out
	RetryCheckInterval   time.Duration // How often to check for jobs that have failed and need to be retried
}

func NewScheduledJobManager(
	logger *zap.Logger,
	db *gorm.DB,
	jobModel SchedulableJob,
	maxRetry int,
	maxInFlightJobs int,
	queuedTimeout time.Duration,
	inProgressTimeout time.Duration,
	oldJobRetention time.Duration,
	enqueueCheckInterval time.Duration,
	timeoutCheckInterval time.Duration,
	retryCheckInterval time.Duration,
) (*ScheduledJobManager, error) {
	// Check if the job model includes an embedded scheduled job
	if reflect.TypeOf(jobModel).Kind() != reflect.Pointer {
		logger.Error("Job model must be a pointer to a struct", zap.String("job model", reflect.TypeOf(jobModel).String()))
		return nil, errors.New(fmt.Sprintf("job model must be a pointer to a struct, got %s", reflect.TypeOf(jobModel).String()))
	}
	if reflect.TypeOf(jobModel).Elem().Kind() != reflect.Struct {
		logger.Error("Job model must be a pointer to a struct", zap.String("job model", reflect.TypeOf(jobModel).String()))
		return nil, errors.New(fmt.Sprintf("job model must be a pointer to a struct, got %s", reflect.TypeOf(jobModel).String()))
	}

	for i := 0; i < reflect.TypeOf(jobModel).Elem().NumField(); i++ {
		field := reflect.TypeOf(jobModel).Elem().Field(i)
		if field.Anonymous && field.Type == reflect.TypeOf(ScheduledJob{}) {
			break
		}
		if i == reflect.TypeOf(jobModel).Elem().NumField()-1 {
			logger.Error("job model must include an embedded scheduled job", zap.String("job model", reflect.TypeOf(jobModel).String()))
			return nil, errors.New(fmt.Sprintf("job model must include an embedded scheduled job, got %s", reflect.TypeOf(jobModel).String()))
		}
	}

	return &ScheduledJobManager{
		logger:               logger,
		db:                   db,
		jobModel:             jobModel,
		MaxRetry:             maxRetry,
		MaxInFlightJobs:      maxInFlightJobs,
		QueuedTimeout:        queuedTimeout,
		InProgressTimeout:    inProgressTimeout,
		OldJobRetention:      oldJobRetention,
		EnqueueCheckInterval: enqueueCheckInterval,
		TimeoutCheckInterval: timeoutCheckInterval,
		RetryCheckInterval:   retryCheckInterval,
	}, nil
}

func (m *ScheduledJobManager) Start() {
	ten := 10
	concurrency.EnsureRunGoroutine(m.enqueueLoop, &ten, nil)
	concurrency.EnsureRunGoroutine(m.timeoutLoop, &ten, nil)
	concurrency.EnsureRunGoroutine(m.retryLoop, &ten, nil)
}

func (m *ScheduledJobManager) AddJob(job SchedulableJob) error {
	// Check if the job type is the same as the job model
	if reflect.TypeOf(job) != reflect.TypeOf(m.jobModel) {
		m.logger.Error("Job type does not match job model", zap.String("job type", reflect.TypeOf(job).String()), zap.String("job model", reflect.TypeOf(m.jobModel).String()))
		return errors.New(fmt.Sprintf("job type does not match this manager's job model, expected %s got %s", reflect.TypeOf(m.jobModel).String(), reflect.TypeOf(job).String()))
	}

	err := m.db.Model(m.jobModel).Create(job).Error
	if err != nil {
		m.logger.Error("Failed to create job", zap.Error(err), zap.String("model name", reflect.TypeOf(m.jobModel).String()))
		return err
	}
	return nil
}

func (m *ScheduledJobManager) SetJobInProgress(job SchedulableJob) error {
	// Check if the job type is the same as the job model
	if reflect.TypeOf(job) != reflect.TypeOf(m.jobModel) {
		m.logger.Error("job type does not match job model", zap.String("job type", reflect.TypeOf(job).String()), zap.String("job model", reflect.TypeOf(m.jobModel).String()))
		return errors.New(fmt.Sprintf("job type does not match this manager's job model, expected %s got %s", reflect.TypeOf(m.jobModel).String(), reflect.TypeOf(job).String()))
	}

	err := m.db.Model(m.jobModel).Where("id = ?", job.GetScheduledJob().ID).
		Where("status = ?", ScheduledJobStatusQueued). // Only set jobs that are queued to in progress so in case of out of order updates we don't set a job that is already in final state to in progress
		Set("status", ScheduledJobStatusInProgress).
		Set("in_progressed_at", time.Now()).Error
	if err != nil {
		m.logger.Error("Failed to set job in progress", zap.Error(err), zap.String("model name", reflect.TypeOf(m.jobModel).String()))
		return err
	}
	return nil
}

func (m *ScheduledJobManager) SetJobResult(job SchedulableJob, result ScheduledJobStatus, failureMessage string) error {
	// Check if the job type is the same as the job model
	if reflect.TypeOf(job) != reflect.TypeOf(m.jobModel) {
		m.logger.Error("Job type does not match job model", zap.String("job type", reflect.TypeOf(job).String()), zap.String("job model", reflect.TypeOf(m.jobModel).String()))
		return errors.New(fmt.Sprintf("job type does not match this manager's job model, expected %s got %s", reflect.TypeOf(m.jobModel).String(), reflect.TypeOf(job).String()))
	}

	err := m.db.Model(m.jobModel).Where("id = ?", job.GetScheduledJob().ID).
		Where("status IN ?", []ScheduledJobStatus{
			ScheduledJobStatusQueued,
			ScheduledJobStatusInProgress,
		}). // Only set jobs that are queued or in progress to final status (queue in case of updates reaching us out of order)
		Set("status", result).
		Set("failure_message", failureMessage).Error
	if err != nil {
		m.logger.Error("Failed to set job result", zap.Error(err), zap.String("model name", reflect.TypeOf(m.jobModel).String()))
		return err
	}
	return nil
}

func (m *ScheduledJobManager) enqueueLoop() {
	t := time.NewTicker(m.EnqueueCheckInterval)
	defer t.Stop()

	for ; ; <-t.C {
		m.enqueue()
		m.cleanup()
	}
}

func (m *ScheduledJobManager) enqueue() {
	var currentInFlightJobs int64
	err := m.db.Model(m.jobModel).Where("status IN ?", []ScheduledJobStatus{
		ScheduledJobStatusQueued,
		ScheduledJobStatusInProgress,
	}).Count(&currentInFlightJobs).Error
	if err != nil {
		m.logger.Error("Failed to count in flight jobs", zap.Error(err), zap.String("model name", reflect.TypeOf(m.jobModel).String()))
		return
	}

	if int(currentInFlightJobs) >= m.MaxInFlightJobs {
		m.logger.Debug("Max in flight jobs reached ignoring this enqueue loop cycle", zap.Int64("current in flight jobs", currentInFlightJobs), zap.Int("max in flight jobs", m.MaxInFlightJobs), zap.String("model name", reflect.TypeOf(m.jobModel).String()))
		return
	}

	var jobs []SchedulableJob
	err = m.db.Model(m.jobModel).Where("status = ?", ScheduledJobStatusCreated).
		Limit(m.MaxInFlightJobs - int(currentInFlightJobs)).Order("created_at asc").Find(&jobs).Error
	if err != nil {
		m.logger.Error("Failed to fetch jobs to enqueue", zap.Error(err), zap.String("model name", reflect.TypeOf(m.jobModel).String()))
		return
	}

	for _, job := range jobs {
		err := job.Enqueue()
		if err != nil {
			m.logger.Error("Failed to enqueue job", zap.Error(err), zap.String("model name", reflect.TypeOf(m.jobModel).String()))
			continue
		}
		err = m.db.Model(m.jobModel).
			Where("id = ?", job.GetScheduledJob().ID).
			Where("status = ?", ScheduledJobStatusCreated). // Only set jobs that are created to queued
			Set("queued_at", time.Now()).Error
		if err != nil {
			m.logger.Error("Failed to update job queued_at", zap.Error(err), zap.String("model name", reflect.TypeOf(m.jobModel).String()))
			continue
		}
	}
}

func (m *ScheduledJobManager) cleanup() {
	err := m.db.Model(m.jobModel).Where("created_at < ?", time.Now().Add(-m.OldJobRetention)).
		Unscoped().Delete(m.jobModel).Error
	if err != nil {
		m.logger.Error("Failed to cleanup jobs", zap.Error(err), zap.String("model name", reflect.TypeOf(m.jobModel).String()))
		return
	}
}

func (m *ScheduledJobManager) timeoutLoop() {
	t := time.NewTicker(m.TimeoutCheckInterval)
	defer t.Stop()

	for ; ; <-t.C {
		m.timeout()
	}
}

func (m *ScheduledJobManager) timeout() {
	err := m.db.Model(m.jobModel).Where("status = ?", ScheduledJobStatusQueued).
		Where("queued_at < ?", time.Now().Add(-m.QueuedTimeout)).
		Set("status", ScheduledJobStatusTimeout).Error
	if err != nil {
		m.logger.Error("Failed to timeout queued jobs", zap.Error(err), zap.String("model name", reflect.TypeOf(m.jobModel).String()))
		return
	}

	err = m.db.Model(m.jobModel).Where("status = ?", ScheduledJobStatusInProgress).
		Where("in_progressed_at < ?", time.Now().Add(-m.InProgressTimeout)).
		Set("status", ScheduledJobStatusTimeout).Error
	if err != nil {
		m.logger.Error("Failed to timeout in progress jobs", zap.Error(err), zap.String("model name", reflect.TypeOf(m.jobModel).String()))
		return
	}
}

func (m *ScheduledJobManager) retryLoop() {
	t := time.NewTicker(m.RetryCheckInterval)
	defer t.Stop()

	for ; ; <-t.C {
		m.retry()
	}
}

func (m *ScheduledJobManager) retry() {
	err := m.db.Model(m.jobModel).
		Where("status IN ?", []ScheduledJobStatus{ScheduledJobStatusFailed, ScheduledJobStatusTimeout}).
		Where("retry_count < ?", m.MaxRetry).
		Where("updated_at < ?", time.Now().Add(-m.RetryCheckInterval)).
		Set("status", ScheduledJobStatusCreated).
		Set("retry_count", gorm.Expr("retry_count + 1")).Error
	if err != nil {
		m.logger.Error("Failed to retry jobs", zap.Error(err), zap.String("model name", reflect.TypeOf(m.jobModel).String()))
		return
	}
}