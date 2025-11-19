package features

import (
	"context"
	"fmt"
	"sync"

	"github.com/Distributed-Task-Queue-System/internal/queue"
	"github.com/Distributed-Task-Queue-System/pkg/job"
)

// TenantManager manages multi-tenancy
type TenantManager struct {
	tenants       map[string]*Tenant
	mu            sync.RWMutex
	queueManager  *queue.Manager
}

// Tenant represents a tenant/organization
type Tenant struct {
	ID            string
	Name          string
	Queues        []string
	ResourceLimit *ResourceLimit
	Metadata      map[string]interface{}
}

// ResourceLimit defines resource limits for a tenant
type ResourceLimit struct {
	MaxJobs          int
	MaxQueues        int
	MaxWorkers       int
	MaxConcurrency   int
	RateLimit        float64 // Jobs per second
	StorageQuotaMB   int64
}

// NewTenantManager creates a new tenant manager
func NewTenantManager(qm *queue.Manager) *TenantManager {
	return &TenantManager{
		tenants:      make(map[string]*Tenant),
		queueManager: qm,
	}
}

// CreateTenant creates a new tenant
func (tm *TenantManager) CreateTenant(tenant *Tenant) error {
	if tenant.ID == "" {
		return fmt.Errorf("tenant ID is required")
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.tenants[tenant.ID]; exists {
		return fmt.Errorf("tenant already exists: %s", tenant.ID)
	}

	// Set default limits if not provided
	if tenant.ResourceLimit == nil {
		tenant.ResourceLimit = &ResourceLimit{
			MaxJobs:        10000,
			MaxQueues:      10,
			MaxWorkers:     100,
			MaxConcurrency: 10,
			RateLimit:      100, // 100 jobs/sec
			StorageQuotaMB: 1024, // 1GB
		}
	}

	tm.tenants[tenant.ID] = tenant

	// Create tenant-specific default queue
	defaultQueue := &job.QueueConfig{
		Name:          fmt.Sprintf("%s_default", tenant.ID),
		MaxWorkers:    tenant.ResourceLimit.MaxWorkers,
		MaxRetries:    3,
		RetryStrategy: job.RetryStrategyExponential,
		Priority:      job.PriorityNormal,
	}

	ctx := context.Background()
	if err := tm.queueManager.RegisterQueue(ctx, defaultQueue); err != nil {
		return fmt.Errorf("failed to create default queue: %w", err)
	}

	tenant.Queues = []string{defaultQueue.Name}

	return nil
}

// GetTenant retrieves a tenant by ID
func (tm *TenantManager) GetTenant(tenantID string) (*Tenant, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tenant, exists := tm.tenants[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant not found: %s", tenantID)
	}

	return tenant, nil
}

// ListTenants returns all tenants
func (tm *TenantManager) ListTenants() []*Tenant {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tenants := make([]*Tenant, 0, len(tm.tenants))
	for _, tenant := range tm.tenants {
		tenants = append(tenants, tenant)
	}

	return tenants
}

// DeleteTenant deletes a tenant
func (tm *TenantManager) DeleteTenant(tenantID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tenant, exists := tm.tenants[tenantID]
	if !exists {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}

	// Delete tenant queues
	ctx := context.Background()
	for _, queueName := range tenant.Queues {
		tm.queueManager.DeleteQueue(ctx, queueName)
	}

	delete(tm.tenants, tenantID)

	return nil
}

// CheckResourceLimit checks if a tenant can create more jobs
func (tm *TenantManager) CheckResourceLimit(tenantID string, jobCount int) error {
	tenant, err := tm.GetTenant(tenantID)
	if err != nil {
		return err
	}

	if jobCount >= tenant.ResourceLimit.MaxJobs {
		return fmt.Errorf("tenant job limit exceeded: %d/%d", jobCount, tenant.ResourceLimit.MaxJobs)
	}

	return nil
}

// UpdateResourceLimit updates tenant resource limits
func (tm *TenantManager) UpdateResourceLimit(tenantID string, limits *ResourceLimit) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tenant, exists := tm.tenants[tenantID]
	if !exists {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}

	tenant.ResourceLimit = limits

	return nil
}

// GetTenantQueues returns all queues for a tenant
func (tm *TenantManager) GetTenantQueues(tenantID string) ([]string, error) {
	tenant, err := tm.GetTenant(tenantID)
	if err != nil {
		return nil, err
	}

	return tenant.Queues, nil
}

// AddQueueToTenant adds a queue to a tenant
func (tm *TenantManager) AddQueueToTenant(tenantID, queueName string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tenant, exists := tm.tenants[tenantID]
	if !exists {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}

	if len(tenant.Queues) >= tenant.ResourceLimit.MaxQueues {
		return fmt.Errorf("tenant queue limit exceeded: %d/%d",
			len(tenant.Queues), tenant.ResourceLimit.MaxQueues)
	}

	tenant.Queues = append(tenant.Queues, queueName)

	return nil
}

// TenantContext adds tenant information to context
type tenantContextKey struct{}

// WithTenant adds tenant ID to context
func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, tenantID)
}

// GetTenantFromContext retrieves tenant ID from context
func GetTenantFromContext(ctx context.Context) (string, bool) {
	tenantID, ok := ctx.Value(tenantContextKey{}).(string)
	return tenantID, ok
}
