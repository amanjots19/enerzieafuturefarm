'use client';

import { useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { me } from '@/lib/api/auth';
import { createProduct, getUploadSignature } from '@/lib/api/products';
import { clearToken, getToken, onUnauthorized } from '@/lib/api/token';
import { isApiError } from '@/lib/api/errors';
import { PageShell } from '@/components/PageShell';
import type { AdminImage } from '@/lib/api/types';

/* ----------------------------------------------------------------- helpers */

function parsePaise(rupeeStr: string): number {
  const n = parseFloat(rupeeStr);
  return isNaN(n) ? 0 : Math.round(n * 100);
}

function parseIntOr0(s: string): number {
  const n = parseInt(s, 10);
  return isNaN(n) ? 0 : n;
}

/* ---------------------------------------------------------------- form state */

interface Draft {
  productId: string;
  name: string;
  family: string;
  form: string;
  stat: string;
  stat2: string;
  blurb: string;
  grad: string;
  positionStr: string;
  mrpStr: string;
  priceStr: string;
  stockStr: string;
  active: boolean;
  ratingScoreStr: string;
  ratingCountStr: string;
  badges: { title: string; subtitle: string }[];
  nutritionServingSize: string;
  nutritionRows: { key: string; value: string }[];
  images: AdminImage[];
}

const initial: Draft = {
  productId: '',
  name: '',
  family: '',
  form: '',
  stat: '',
  stat2: '',
  blurb: '',
  grad: '',
  positionStr: '0',
  mrpStr: '',
  priceStr: '',
  stockStr: '0',
  active: false,
  ratingScoreStr: '0',
  ratingCountStr: '0',
  badges: [],
  nutritionServingSize: '',
  nutritionRows: [],
  images: [],
};

function buildBody(d: Draft): Record<string, unknown> {
  return {
    id: d.productId,
    name: d.name,
    family: d.family,
    form: d.form,
    stat: d.stat,
    stat2: d.stat2,
    blurb: d.blurb,
    grad: d.grad,
    position: parseIntOr0(d.positionStr),
    mrp: parsePaise(d.mrpStr),
    price: parsePaise(d.priceStr),
    stock: parseIntOr0(d.stockStr),
    active: d.active,
    rating: {
      score: parseFloat(d.ratingScoreStr) || 0,
      count: parseIntOr0(d.ratingCountStr),
    },
    badges: d.badges,
    nutrition:
      d.nutritionServingSize || d.nutritionRows.length > 0
        ? { servingSize: d.nutritionServingSize, rows: d.nutritionRows }
        : null,
    images: d.images,
    // discountPercent and soldOut intentionally omitted — computed by server;
    // sending them causes a 422.
  };
}

/* ------------------------------------------------------------------ page */

export default function NewProductPage() {
  const router = useRouter();
  const [ready, setReady] = useState(false);
  const [email, setEmail] = useState('');
  const [draft, setDraft] = useState<Draft>(initial);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [idError, setIdError] = useState('');
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState('');
  const fileRef = useRef<HTMLInputElement>(null);

  /* auth guard */
  useEffect(() => {
    const tok = getToken();
    if (!tok) { router.replace('/login'); return; }
    let cancelled = false;
    me()
      .then((d) => { if (!cancelled) { setEmail(d.email); setReady(true); } })
      .catch(() => { clearToken(); if (!cancelled) router.replace('/login'); });
    return () => { cancelled = true; };
  }, [router]);

  useEffect(() => {
    onUnauthorized(() => { clearToken(); router.replace('/login'); });
  }, [router]);

  /* ----------------------------------------------------------- field helpers */

  function set<K extends keyof Draft>(field: K, value: Draft[K]) {
    setDraft((prev) => ({ ...prev, [field]: value }));
  }

  /* badges */
  function addBadge() {
    setDraft((p) => ({ ...p, badges: [...p.badges, { title: '', subtitle: '' }] }));
  }
  function removeBadge(i: number) {
    setDraft((p) => ({ ...p, badges: p.badges.filter((_, idx) => idx !== i) }));
  }
  function setBadgeField(i: number, field: 'title' | 'subtitle', value: string) {
    setDraft((p) => ({
      ...p,
      badges: p.badges.map((b, idx) => (idx === i ? { ...b, [field]: value } : b)),
    }));
  }

  /* nutrition rows */
  function addNutritionRow() {
    setDraft((p) => ({ ...p, nutritionRows: [...p.nutritionRows, { key: '', value: '' }] }));
  }
  function removeNutritionRow(i: number) {
    setDraft((p) => ({ ...p, nutritionRows: p.nutritionRows.filter((_, idx) => idx !== i) }));
  }
  function setNutritionRowField(i: number, field: 'key' | 'value', value: string) {
    setDraft((p) => ({
      ...p,
      nutritionRows: p.nutritionRows.map((r, idx) => (idx === i ? { ...r, [field]: value } : r)),
    }));
  }

  /* images */
  function makePrimary(index: number) {
    setDraft((p) => {
      const imgs = [...p.images];
      const removed = imgs.splice(index, 1);
      const selected = removed[0];
      if (!selected) return p;
      return { ...p, images: [selected, ...imgs] };
    });
  }
  function removeImage(index: number) {
    setDraft((p) => ({ ...p, images: p.images.filter((_, idx) => idx !== index) }));
  }
  function setImageAlt(index: number, alt: string) {
    setDraft((p) => ({
      ...p,
      images: p.images.map((img, idx) => (idx === index ? { ...img, alt } : img)),
    }));
  }

  async function handleImageUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    // Reset so the same file can be picked again after an error.
    e.target.value = '';
    setUploadError('');
    setUploading(true);
    try {
      // Step 1: get signing params from our server.
      let sig;
      try {
        sig = await getUploadSignature();
      } catch (err) {
        if (isApiError(err) && err.code === 'INTERNAL') {
          throw new Error(
            'Cloudinary is not configured on this server. Add CLOUDINARY_* variables to enerzia-be/.env.',
          );
        }
        throw new Error(
          isApiError(err)
            ? `Could not get upload signature: ${err.message}`
            : 'Could not reach the server to prepare the upload.',
        );
      }

      // Step 2: upload the file directly to Cloudinary.
      const fd = new FormData();
      fd.append('file', file);
      fd.append('api_key', sig.apiKey);
      fd.append('timestamp', String(sig.timestamp));
      fd.append('folder', sig.folder);
      fd.append('signature', sig.signature);

      let res: Response;
      try {
        res = await fetch(`https://api.cloudinary.com/v1_1/${sig.cloudName}/image/upload`, {
          method: 'POST',
          body: fd,
        });
      } catch {
        throw new Error('Could not reach Cloudinary. Check your connection.');
      }

      if (!res.ok) {
        throw new Error(`Cloudinary rejected the upload (HTTP ${res.status}).`);
      }

      const data = (await res.json()) as { secure_url?: string; public_id?: string };
      const url = data.secure_url;
      const publicId = data.public_id;
      if (!url || !publicId) {
        throw new Error('Cloudinary response was missing secure_url or public_id.');
      }

      setDraft((p) => ({ ...p, images: [...p.images, { url, publicId, alt: '' }] }));
    } catch (err) {
      setUploadError(err instanceof Error ? err.message : 'Upload failed.');
    } finally {
      setUploading(false);
    }
  }

  /* ---------------------------------------------------------- form submit */

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    setIdError('');
    setSaving(true);
    try {
      const product = await createProduct(buildBody(draft));
      router.push(`/products/${product.id}`);
    } catch (err) {
      if (isApiError(err)) {
        if (err.code === 'CONFLICT') {
          // 409 — id already exists: show against the id field, not page-level
          setIdError(err.message);
        } else if (err.fields.length > 0) {
          setError(err.fields[0]?.message ?? err.message);
        } else {
          setError(err.message);
        }
      } else {
        setError('Could not save. Check your connection and try again.');
      }
    } finally {
      setSaving(false);
    }
  }

  if (!ready) return <div className="loading-center">Loading…</div>;

  return (
    <PageShell email={email}>
      <main className="page-content">
        <div className="list-toolbar">
          <h2 className="page-title">New product</h2>
        </div>

        <form onSubmit={handleSubmit} noValidate>
          {/* -------- Identity -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Identity</h3>

            <div className="form-field">
              <label htmlFor="f-id">Product ID</label>
              <input
                id="f-id"
                type="text"
                value={draft.productId}
                onChange={(e) => { set('productId', e.target.value); setIdError(''); }}
              />
              <span className="form-hint">
                Permanent — appears in every URL, cart line, and order. Cannot be changed after creation.
              </span>
              {idError && <span className="field-error">{idError}</span>}
            </div>

            <div className="form-field">
              <label htmlFor="f-name">Name</label>
              <input id="f-name" type="text" value={draft.name}
                onChange={(e) => set('name', e.target.value)} />
            </div>

            <div className="form-row">
              <div className="form-field">
                <label htmlFor="f-family">Family</label>
                <input id="f-family" type="text" value={draft.family}
                  onChange={(e) => set('family', e.target.value)} />
              </div>
              <div className="form-field">
                <label htmlFor="f-form">Form</label>
                <select id="f-form" value={draft.form}
                  onChange={(e) => set('form', e.target.value)}>
                  <option value="">Select…</option>
                  <option value="Powder">Powder</option>
                  <option value="Tablets">Tablets</option>
                  <option value="Bundle">Bundle</option>
                </select>
              </div>
            </div>

            <div className="form-field">
              <label htmlFor="f-stat">Stat</label>
              <input id="f-stat" type="text" value={draft.stat}
                onChange={(e) => set('stat', e.target.value)} />
            </div>

            <div className="form-field">
              <label htmlFor="f-stat2">Stat 2</label>
              <input id="f-stat2" type="text" value={draft.stat2}
                onChange={(e) => set('stat2', e.target.value)} />
            </div>

            <div className="form-field">
              <label htmlFor="f-blurb">Blurb</label>
              <textarea id="f-blurb" rows={4} value={draft.blurb}
                onChange={(e) => set('blurb', e.target.value)} />
            </div>

            <div className="form-field">
              <label htmlFor="f-grad">Gradient</label>
              <input id="f-grad" type="text" value={draft.grad}
                onChange={(e) => set('grad', e.target.value)}
                placeholder="radial-gradient(…)" />
            </div>
          </div>

          {/* -------- Pricing -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Pricing</h3>
            <p className="form-section-note">Enter amounts in rupees. They are stored as paise (× 100).</p>
            <div className="form-row">
              <div className="form-field">
                <label htmlFor="f-mrp">MRP (₹)</label>
                <input id="f-mrp" type="number" min="0" step="0.01"
                  value={draft.mrpStr} placeholder="e.g. 470.00"
                  onChange={(e) => set('mrpStr', e.target.value)} />
              </div>
              <div className="form-field">
                <label htmlFor="f-price">Price (₹)</label>
                <input id="f-price" type="number" min="0" step="0.01"
                  value={draft.priceStr} placeholder="e.g. 380.00"
                  onChange={(e) => set('priceStr', e.target.value)} />
              </div>
            </div>
          </div>

          {/* -------- Inventory -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Inventory</h3>
            <div className="form-row">
              <div className="form-field">
                <label htmlFor="f-stock">Stock</label>
                <input id="f-stock" type="number" min="0" step="1"
                  value={draft.stockStr}
                  onChange={(e) => set('stockStr', e.target.value)} />
              </div>
              <div className="form-field">
                <label htmlFor="f-position">Position</label>
                <input id="f-position" type="number" min="0" step="1"
                  value={draft.positionStr}
                  onChange={(e) => set('positionStr', e.target.value)} />
              </div>
            </div>
            <div className="form-field">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  checked={draft.active}
                  onChange={(e) => set('active', e.target.checked)}
                />
                Active (visible in shop)
              </label>
              <span className="form-hint">
                Unchecked means this product stays hidden from the shop until you tick this and save.
              </span>
            </div>
          </div>

          {/* -------- Rating -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Rating</h3>
            <div className="form-row">
              <div className="form-field">
                <label htmlFor="f-rscore">Score (0–5)</label>
                <input id="f-rscore" type="number" min="0" max="5" step="0.1"
                  value={draft.ratingScoreStr}
                  onChange={(e) => set('ratingScoreStr', e.target.value)} />
              </div>
              <div className="form-field">
                <label htmlFor="f-rcount">Review count</label>
                <input id="f-rcount" type="number" min="0" step="1"
                  value={draft.ratingCountStr}
                  onChange={(e) => set('ratingCountStr', e.target.value)} />
              </div>
            </div>
          </div>

          {/* -------- Images -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Images</h3>
            <p className="form-section-note">Index 0 is the primary shot the shop renders.</p>

            {draft.images.length > 0 && (
              <div className="image-grid">
                {draft.images.map((img, i) => (
                  <div key={img.publicId || i} className={`image-item${i === 0 ? ' image-primary' : ''}`}>
                    {/* plain <img>, not next/image — no remotePatterns config needed for internal tool */}
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img src={img.url} alt={img.alt || ''} className="image-thumb" />
                    {i === 0 && <span className="badge-primary">Primary</span>}
                    <div className="image-meta">
                      <input
                        type="text"
                        placeholder="Alt text"
                        value={img.alt}
                        onChange={(e) => setImageAlt(i, e.target.value)}
                        className="image-alt-input"
                      />
                    </div>
                    <div className="image-actions">
                      {i > 0 && (
                        <button type="button" className="btn btn-ghost btn-xs"
                          onClick={() => makePrimary(i)}>
                          Make primary
                        </button>
                      )}
                      <button type="button" className="btn btn-ghost btn-xs btn-danger-ghost"
                        onClick={() => removeImage(i)}>
                        Remove
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}

            {draft.images.length < 5 ? (
              <div className="upload-area">
                <input
                  ref={fileRef}
                  type="file"
                  accept="image/*"
                  style={{ display: 'none' }}
                  onChange={handleImageUpload}
                  disabled={uploading}
                />
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={() => fileRef.current?.click()}
                  disabled={uploading}
                >
                  {uploading ? 'Uploading…' : 'Add image'}
                </button>
                {uploading && <span className="upload-pending"> Uploading to Cloudinary…</span>}
              </div>
            ) : (
              <p className="form-hint">Maximum 5 images reached.</p>
            )}

            {uploadError && <div className="error-msg" style={{ marginTop: '0.5rem' }}>{uploadError}</div>}
          </div>

          {/* -------- Badges -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Badges</h3>
            {draft.badges.map((b, i) => (
              <div key={i} className="dynamic-row">
                <input type="text" placeholder="Title" value={b.title}
                  onChange={(e) => setBadgeField(i, 'title', e.target.value)} />
                <input type="text" placeholder="Subtitle" value={b.subtitle}
                  onChange={(e) => setBadgeField(i, 'subtitle', e.target.value)} />
                <button type="button" className="btn btn-ghost btn-xs btn-danger-ghost"
                  onClick={() => removeBadge(i)}>
                  ×
                </button>
              </div>
            ))}
            <button type="button" className="btn btn-ghost btn-sm" onClick={addBadge}>
              Add badge
            </button>
          </div>

          {/* -------- Nutrition -------- */}
          <div className="form-section">
            <h3 className="form-section-title">Nutrition</h3>
            <div className="form-field">
              <label htmlFor="f-serving">Serving size</label>
              <input id="f-serving" type="text" value={draft.nutritionServingSize}
                placeholder="e.g. 5 g"
                onChange={(e) => set('nutritionServingSize', e.target.value)} />
            </div>
            {draft.nutritionRows.map((r, i) => (
              <div key={i} className="dynamic-row">
                <input type="text" placeholder="Nutrient" value={r.key}
                  onChange={(e) => setNutritionRowField(i, 'key', e.target.value)} />
                <input type="text" placeholder="Amount" value={r.value}
                  onChange={(e) => setNutritionRowField(i, 'value', e.target.value)} />
                <button type="button" className="btn btn-ghost btn-xs btn-danger-ghost"
                  onClick={() => removeNutritionRow(i)}>
                  ×
                </button>
              </div>
            ))}
            <button type="button" className="btn btn-ghost btn-sm" onClick={addNutritionRow}>
              Add row
            </button>
          </div>

          {/* -------- Actions -------- */}
          {error && <div className="error-msg" role="alert">{error}</div>}
          <div className="save-bar">
            <button type="submit" className="btn btn-primary" disabled={saving}>
              {saving ? 'Creating…' : 'Create product'}
            </button>
          </div>
        </form>
      </main>
    </PageShell>
  );
}
