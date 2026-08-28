package tenantcache

import (
	"context"
	"fmt"
	"time"

	"cortex/backend/internal/domain"
	"cortex/backend/internal/rediscoord"
	"github.com/google/uuid"
)

type Redis struct {
	client *rediscoord.Client
}

func NewRedis(client *rediscoord.Client) *Redis {
	return &Redis{client: client}
}

func (r *Redis) InvalidateDeletedTenant(ctx context.Context, principal domain.Principal, publicTemplateIDs []uuid.UUID) {
	if r == nil || r.client == nil {
		return
	}
	_ = r.client.Set(ctx, fmt.Sprintf("cortex:auth:tenant-version:%s", principal.TenantID), "deleted", 24*time.Hour)
	if principal.AuthCacheKey != "" {
		_ = r.client.Delete(ctx, principal.AuthCacheKey)
	}
	for _, id := range publicTemplateIDs {
		_ = r.client.DeleteTemplateProjections(ctx, id.String())
	}
}
