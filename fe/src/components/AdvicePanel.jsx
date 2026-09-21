import React, { useEffect, useMemo, useState } from 'react';
import { api } from '../services/api';

const PROFILE_KEY = 'fish-pond-profile-v2';

const SPECIES = [
  { id: 'ca_chinh', label: 'Cá chình' },
  { id: 'koi', label: 'Koi' },
  { id: 'ca_vang', label: 'Cá vàng' },
  { id: 'general', label: 'Cá cảnh (hồ nhỏ)' },
  { id: 'discus', label: 'Cá đĩa' },
  { id: 'neon', label: 'Neon' },
];

const DEFAULT_PROFILE = {
  volume_m3: 9,
  species: 'ca_chinh',
  has_filter: false,
};

function loadProfile() {
  try {
    const raw = localStorage.getItem(PROFILE_KEY);
    if (!raw) return DEFAULT_PROFILE;
    return { ...DEFAULT_PROFILE, ...JSON.parse(raw) };
  } catch {
    return DEFAULT_PROFILE;
  }
}

function formatDelta(metric, delta) {
  if (delta == null) return '—';
  const sign = delta > 0 ? '+' : '';
  if (metric === 'ph') return `${sign}${Number(delta).toFixed(2)}`;
  if (metric === 'tds') return `${sign}${Math.round(delta)} ppm`;
  if (metric === 'turbidity') return `${sign}${Number(delta).toFixed(1)} NTU`;
  return `${sign}${Number(delta).toFixed(1)} °C`;
}

function slopeLabel(slope) {
  if (slope === 'rising') return 'đang tăng';
  if (slope === 'falling') return 'đang giảm';
  if (slope === 'stable') return 'ổn định';
  return 'chưa rõ';
}

function overallBadge(overall) {
  if (overall === 'ok') return { text: 'Ổn định', cls: 'badge-ideal' };
  if (overall === 'warning') return { text: 'Cần xử lý', cls: 'badge-warning' };
  if (overall === 'danger') return { text: 'Khẩn', cls: 'badge-danger' };
  return { text: 'Chưa đủ dữ liệu', cls: 'badge-warning' };
}

const METRIC_NAME = {
  temperature: 'Nhiệt độ',
  ph: 'pH',
  turbidity: 'Độ đục',
  tds: 'TDS',
};

export default function AdvicePanel({ deviceId, onPump, pondConfig, onPondConfigChange }) {
  const [profile, setProfile] = useState(loadProfile);
  const [advice, setAdvice] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!pondConfig) return;
    setProfile((prev) => ({
      ...prev,
      volume_m3: pondConfig.volume_m3 ?? prev.volume_m3,
      species: pondConfig.species || prev.species,
      has_filter: !!pondConfig.has_filter,
    }));
  }, [pondConfig]);

  useEffect(() => {
    localStorage.setItem(PROFILE_KEY, JSON.stringify(profile));
  }, [profile]);

  useEffect(() => {
    setAdvice(null);
    setError('');
  }, [deviceId]);

  const badge = useMemo(() => overallBadge(advice?.overall), [advice]);

  const analyze = async () => {
    if (!deviceId) return;
    setLoading(true);
    setError('');
    try {
      const m3 = Number(profile.volume_m3);
      const saved = await api.updatePondConfig({
        volume_m3: Number.isFinite(m3) ? m3 : 9,
        species: profile.species || 'ca_chinh',
        has_filter: !!profile.has_filter,
      });
      if (saved && onPondConfigChange) onPondConfigChange(saved);
      const data = await api.getAdvice(deviceId, {
        volume_l: (Number.isFinite(m3) ? m3 : 9) * 1000,
        species: profile.species || 'ca_chinh',
        has_filter: !!profile.has_filter,
      });
      setAdvice(data);
    } catch (e) {
      setAdvice(null);
      setError(e.message || 'Không phân tích được');
    } finally {
      setLoading(false);
    }
  };

  const handlePump = async (target, state) => {
    if (!onPump) return;
    await onPump(target, state);
  };

  return (
    <div className="glass-card advice-card">
      <div className="section-title">
        <svg width="18" height="18" fill="var(--accent)" viewBox="0 0 24 24">
          <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-6h2v6zm0-8h-2V7h2v2z" />
        </svg>
        Phân Tích Hồ & Đề Xuất
        {advice && (
          <span className={`sensor-badge ${badge.cls}`} style={{ marginLeft: 'auto', textTransform: 'none' }}>
            {badge.text}
          </span>
        )}
      </div>

      <p className="advice-lead">
        Phân tích cần OpenAI. Không kết nối hoặc không gọi được API thì không ra đề xuất.
      </p>

      <div className="advice-profile">
        <label>
          Thể tích hồ (khối / m³)
          <input
            type="number"
            min="0.1"
            step="0.1"
            value={profile.volume_m3}
            onChange={(e) => setProfile((p) => ({ ...p, volume_m3: e.target.value }))}
          />
          <span className="advice-field-hint">
            {Number(profile.volume_m3) > 0
              ? `= ${(Number(profile.volume_m3) * 1000).toLocaleString('vi-VN')} lít`
              : '1 khối = 1000 lít'}
          </span>
        </label>
        <label>
          Loài cá
          <select
            value={profile.species}
            onChange={(e) => setProfile((p) => ({ ...p, species: e.target.value }))}
          >
            {SPECIES.map((s) => (
              <option key={s.id} value={s.id}>{s.label}</option>
            ))}
          </select>
          <span className="advice-field-hint">Ngưỡng pH / TDS / nhiệt theo loài</span>
        </label>
        <label className="advice-check" title="Bể lọc, lọc trống, bể lắng — không phải máy sục khí">
          <input
            type="checkbox"
            checked={!!profile.has_filter}
            onChange={(e) => setProfile((p) => ({ ...p, has_filter: e.target.checked }))}
          />
          <span>
            Có hệ thống lọc
            <span className="advice-field-hint">Bể lọc / lắng. Không phải sục khí.</span>
          </span>
        </label>
        <button className="btn btn-primary" onClick={analyze} disabled={loading || !deviceId}>
          {loading ? 'Đang phân tích…' : 'Phân tích xu hướng'}
        </button>
      </div>

      {pondConfig?.badges && (
        <div className="advice-findings" style={{ marginTop: '-4px' }}>
          {['ph', 'turbidity', 'tds', 'temperature'].map((key) => (
            pondConfig.badges[key]?.ok ? (
              <span key={key} className="sensor-badge badge-ideal" style={{ textTransform: 'none' }}>
                {pondConfig.badges[key].ok}
              </span>
            ) : null
          ))}
        </div>
      )}

      {error && <div className="advice-error">{error}</div>}

      {advice && (
        <>
          <p className="advice-summary">{advice.summary}</p>

          {!advice.ready && (
            <div className="advice-banner">
              Chưa đủ chuỗi đo để kết luận chắc (cần ≥ 3 mẫu / chỉ số, trải ≥ 1 giờ). Bật lịch auto rồi phân tích lại.
              {advice.reason ? ` (${advice.reason})` : ''}
            </div>
          )}

          {advice.stale && (
            <div className="advice-banner">
              Mẫu cuối đã cũ{advice.latest_at ? ` (${new Date(advice.latest_at).toLocaleString('vi-VN')})` : ''}. Xu hướng tính trên phiên đo gần nhất, không phải 24h đồng hồ tường.
            </div>
          )}

          <div className="advice-meta">
            {advice.used_llm ? 'Đã phân tích qua OpenAI' : 'Chưa gọi OpenAI (thiếu chuỗi đo)'}
            {advice.sample_window ? ` · cửa sổ ${advice.sample_window}` : ''}
          </div>

          <div className="advice-trend-grid">
            {['ph', 'tds', 'turbidity', 'temperature'].map((key) => {
              const tr = advice.trend?.[key] || {};
              return (
                <div key={key} className="advice-trend-item">
                  <div className="advice-trend-name">{METRIC_NAME[key]}</div>
                  <div className="advice-trend-now">
                    {tr.current == null ? '—' : key === 'tds' ? Math.round(tr.current) : tr.current}
                  </div>
                  <div className="advice-trend-slope">{slopeLabel(tr.slope)}</div>
                  <div className="advice-trend-deltas">
                    1h {formatDelta(key, tr.delta_1h)} · 6h {formatDelta(key, tr.delta_6h)}
                  </div>
                  <div className="advice-trend-n">{tr.n || 0} mẫu / 24h</div>
                </div>
              );
            })}
          </div>

          {advice.findings?.length > 0 && (
            <div className="advice-findings">
              {advice.findings.map((f) => (
                <span
                  key={f.metric}
                  className={`sensor-badge ${f.status === 'ok' ? 'badge-ideal' : f.status === 'missing' ? 'badge-warning' : 'badge-danger'}`}
                  style={{ textTransform: 'none' }}
                >
                  {f.label}
                </span>
              ))}
            </div>
          )}

          <ol className="advice-actions">
            {(advice.actions || []).map((a) => (
              <li key={`${a.priority}-${a.type}`}>
                <div className="advice-action-title">{a.title}</div>
                <div className="advice-action-detail">{a.detail}</div>
                {a.type === 'water_change_percent' && onPump && (
                  <div className="advice-action-hw">
                    <button className="btn btn-secondary" type="button" onClick={() => handlePump('drain', true)}>
                      Mở van xả
                    </button>
                    <button className="btn btn-secondary" type="button" onClick={() => handlePump('inlet', true)}>
                      Mở van nạp
                    </button>
                    <span className="advice-action-hw-note">
                      Tắt van thủ công khi đủ {a.amount_pct || 20}%. Không xả cạn hồ.
                    </span>
                  </div>
                )}
              </li>
            ))}
          </ol>

          {advice.do_not?.length > 0 && (
            <div className="advice-donot">
              <div className="advice-donot-title">Không nên</div>
              <ul>
                {advice.do_not.map((d, i) => (
                  <li key={i}>{d}</li>
                ))}
              </ul>
            </div>
          )}

          {advice.limitations && (
            <p className="advice-limit">{advice.limitations}</p>
          )}
        </>
      )}
    </div>
  );
}
