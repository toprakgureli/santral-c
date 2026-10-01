package store

import (
	"context"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// ApprovedTemplate finds an approved template of a business account by
// name, in the given language or in any language when lang is empty.
func (r *Repository) ApprovedTemplate(ctx context.Context, wabaID, name, lang string) (*models.WATemplate, error) {
	var tpl models.WATemplate
	if err := r.db.WithContext(ctx).Where("waba_id = ? AND name = ? AND status = 'APPROVED'", wabaID, name).
		Where("language = ? OR ? = ''", lang, lang).First(&tpl).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

// ApprovedTemplateAnyLanguage finds an approved template of a business
// account by name, in whichever language comes first.
func (r *Repository) ApprovedTemplateAnyLanguage(ctx context.Context, wabaID, name string) (*models.WATemplate, error) {
	var tpl models.WATemplate
	if err := r.db.WithContext(ctx).Where("waba_id = ? AND name = ? AND status = 'APPROVED'", wabaID, name).First(&tpl).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

// ApprovedTemplateIn finds an approved template of a business account by
// name and language.
func (r *Repository) ApprovedTemplateIn(ctx context.Context, wabaID, name, lang string) (*models.WATemplate, error) {
	var tpl models.WATemplate
	if err := r.db.WithContext(ctx).Where("waba_id = ? AND name = ? AND language = ? AND status = 'APPROVED'", wabaID, name, lang).First(&tpl).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

// TemplatesOf lists a business account's templates by name and language;
// with approvedOnly, only the approved ones.
func (r *Repository) TemplatesOf(ctx context.Context, wabaID string, approvedOnly bool) ([]models.WATemplate, error) {
	q := r.db.WithContext(ctx).Where("waba_id = ?", wabaID)
	if approvedOnly {
		q = q.Where("status = 'APPROVED'")
	}
	var list []models.WATemplate
	if err := q.Order("name, language").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// SetTemplateFill stores what fills each blank of a template, as a JSON list.
func (r *Repository) SetTemplateFill(ctx context.Context, id uint, fill string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_templates SET fill = ? WHERE id = ?", fill, id).Error
}

// SetTemplateFillByName stores what fills each blank of a template found
// by account, name and language, as a JSON list.
func (r *Repository) SetTemplateFillByName(ctx context.Context, wabaID, name, lang, fill string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_templates SET fill = ? WHERE waba_id = ? AND name = ? AND language = ?", fill, wabaID, name, lang).Error
}

// ReloadTemplateByName reads a template again into t, found by t's
// account, name and language.
func (r *Repository) ReloadTemplateByName(ctx context.Context, t *models.WATemplate) error {
	return r.db.WithContext(ctx).Where("waba_id = ? AND name = ? AND language = ?", t.WABAID, t.Name, t.Language).First(t).Error
}

// SyncedTemplate is a template as Meta reports it.
type SyncedTemplate struct {
	MetaID         string
	Name           string
	Language       string
	Category       string
	Status         string
	Components     string
	RejectedReason string
	Quality        string
}

// UpsertSyncedTemplate stores a template Meta reported for a business
// account, updating the one with the same name and language.
func (r *Repository) UpsertSyncedTemplate(ctx context.Context, wabaID string, t SyncedTemplate) error {
	return r.db.WithContext(ctx).Exec(`INSERT INTO wa_templates (waba_id, meta_id, name, language, category, status, components, rejected_reason, quality, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, now())
			ON CONFLICT (waba_id, name, language) DO UPDATE SET meta_id = EXCLUDED.meta_id, category = EXCLUDED.category, status = EXCLUDED.status,
				components = EXCLUDED.components, rejected_reason = EXCLUDED.rejected_reason, quality = EXCLUDED.quality, updated_at = now()`,
		wabaID, t.MetaID, t.Name, t.Language, t.Category, t.Status, t.Components, t.RejectedReason, t.Quality).Error
}

// DeleteTemplatesNotIn removes a business account's templates known to
// Meta whose Meta id is not in the list.
func (r *Repository) DeleteTemplatesNotIn(ctx context.Context, wabaID string, metaIDs []string) error {
	return r.db.WithContext(ctx).Exec("DELETE FROM wa_templates WHERE waba_id = ? AND meta_id <> '' AND meta_id NOT IN ?", wabaID, metaIDs).Error
}

// UpsertCreatedTemplate stores a template just sent to Meta for approval,
// updating the one with the same name and language.
func (r *Repository) UpsertCreatedTemplate(ctx context.Context, t *models.WATemplate, createdBy uint) error {
	return r.db.WithContext(ctx).Exec(`INSERT INTO wa_templates (waba_id, meta_id, name, language, category, status, components, created_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, now()) ON CONFLICT (waba_id, name, language) DO UPDATE SET meta_id = EXCLUDED.meta_id, status = EXCLUDED.status,
		components = EXCLUDED.components, category = EXCLUDED.category, updated_at = now()`,
		t.WABAID, t.MetaID, t.Name, t.Language, t.Category, t.Status, t.Components, createdBy).Error
}

// DeleteTemplateByName removes a business account's template in every language.
func (r *Repository) DeleteTemplateByName(ctx context.Context, wabaID, name string) error {
	return r.db.WithContext(ctx).Where("waba_id = ? AND name = ?", wabaID, name).Delete(&models.WATemplate{}).Error
}

// SetTemplateStatus records a template's review result, found by Meta id
// or by name and language.
func (r *Repository) SetTemplateStatus(ctx context.Context, wabaID, metaID, name, lang, status, reason string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_templates SET status = ?, rejected_reason = ?, updated_at = now() WHERE waba_id = ? AND (meta_id = ? OR (name = ? AND language = ?))",
		status, reason, wabaID, metaID, name, lang).Error
}

// SetTemplateQuality records a template's quality score.
func (r *Repository) SetTemplateQuality(ctx context.Context, wabaID, name, lang, quality string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_templates SET quality = ? WHERE waba_id = ? AND name = ? AND language = ?", quality, wabaID, name, lang).Error
}
