package integration

import (
	"database/sql"
	"github.com/google/uuid"
	"github.com/jackc/pgtype"
	"time"
)

type Type string

func (t Type) String() string {
	return string(t)
}

type IntegrationState string

const (
	IntegrationStateActive   IntegrationState = "ACTIVE"
	IntegrationStateInactive IntegrationState = "INACTIVE"
	IntegrationStateArchived IntegrationState = "ARCHIVED"
	IntegrationStateSample   IntegrationState = "SAMPLE_INTEGRATION"
)

type Integration struct {
	IntegrationID   uuid.UUID `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"` // Auto-generated UUID
	ProviderID      string
	Name            string
	IntegrationType Type
	Annotations     pgtype.JSONB
	Labels          pgtype.JSONB

	CredentialID uuid.UUID `gorm:"not null"` // Foreign key to Credential

	State     IntegrationState
	LastCheck *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt sql.NullTime `gorm:"index"`
}
