package job_manager

import (
	"time"
)

type ScheduledJobStatus string

const (
	ScheduledJobStatusCreated    ScheduledJobStatus = "CREATED"
	ScheduledJobStatusQueued     ScheduledJobStatus = "QUEUED"
	ScheduledJobStatusInProgress ScheduledJobStatus = "IN_PROGRESS"
	ScheduledJobStatusTimeout    ScheduledJobStatus = "TIMEOUT"
	ScheduledJobStatusFailed     ScheduledJobStatus = "FAILED"
	ScheduledJobStatusSucceeded  ScheduledJobStatus = "SUCCEEDED"
)

type ScheduledJob struct {
	ID             uint      `gorm:"primarykey"`
	CreatedAt      time.Time `gorm:"index"`
	UpdatedAt      time.Time `gorm:"index:,sort:desc"`
	QueuedAt       time.Time
	InProgressedAt time.Time

	Status         ScheduledJobStatus `gorm:"index:idx_status_composite;index"`
	RetryCount     int
	FailureMessage string
}

type SchedulableJob interface {
	// GetScheduledJob returns the ScheduledJob, which should be embedded in the struct
	GetScheduledJob() ScheduledJob
	Enqueue() error
	SetResult(result ScheduledJobStatus, failureMessage string) error
}
