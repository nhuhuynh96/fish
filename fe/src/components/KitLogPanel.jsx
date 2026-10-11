import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../services/api';

function fmt(v, digits) {
  if (v == null) return '—';
  return Number(v).toFixed(digits);
}

const EMPTY_FORM = { do_mg_l: '', tan_mg_l: '', no2_mg_l: '', no3_mg_l: '', kh_dkh: '', measured_at: '' };

const FIELDS = [
  { key: 'do_mg_l', label: 'Oxy hòa tan DO (mg/L)', max: 20, step: 0.1, placeholder: 'vd. 6.5' },
  { key: 'tan_mg_l', label: 'Amonia tổng NH₄/NH₃ (mg/L)', max: 20, step: 0.01, placeholder: 'vd. 0.25' },
  { key: 'no2_mg_l', label: 'Nitrite NO₂ (mg/L)', max: 20, step: 0.01, placeholder: 'vd. 0.1' },
  { key: 'no3_mg_l', label: 'Nitrate NO₃ (mg/L)', max: 500, step: 1, placeholder: 'vd. 40' },
  { key: 'kh_dkh', label: 'Độ kiềm KH (dKH)', max: 30, step: 0.5, placeholder: 'vd. 7' },
];

function hasValue(v) {
  return v !== '' && v != null && !Number.isNaN(Number(v));
}

export default function KitLogPanel({ deviceId }) {
  const [form, setForm] = useState(EMPTY_FORM);
  const [rows, setRows] = useState([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');

  const load = useCallback(async () => {
    if (!deviceId) return;
    setLoading(true);
    try {
      const list = await api.getKitReadings(deviceId, 50);
      setRows(Array.isArray(list) ? list : []);
    } catch (e) {
      setMessage(e.message || 'Không tải được lịch sử kit');
    } finally {
      setLoading(false);
    }
  }, [deviceId]);

  useEffect(() => {
    setForm(EMPTY_FORM);
    setMessage('');
    load();
  }, [deviceId, load]);

  const handleSave = async (e) => {
    e.preventDefault();
    if (!deviceId) return;
    if (!FIELDS.some((f) => hasValue(form[f.key]))) {
      setMessage('Nhập ít nhất một chỉ số.');
      return;
    }
    setSaving(true);
    setMessage('');
    try {
      let measured_at;
      if (form.measured_at) {
        measured_at = new Date(form.measured_at).toISOString();
      }
      await api.saveKitReading(deviceId, { ...form, measured_at });
      setForm(EMPTY_FORM);
      setMessage('Đã lưu. Phân tích xu hướng trên Dashboard sẽ tự lấy mẫu kit này.');
      await load();
    } catch (err) {
      setMessage(err.message || 'Lưu thất bại');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="glass-card">
      <div className="section-title">
        <svg width="18" height="18" fill="var(--accent)" viewBox="0 0 24 24">
          <path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-2 10H7v-2h10v2zm0-4H7V7h10v2z" />
        </svg>
        Nhật ký test kit
      </div>

      <p className="advice-lead">
        Số từ que kit, không phải cảm biến ESP. Bộ test NH₄/NH₃ cho ra <b>amonia tổng (NH₄ + NH₃, còn gọi là TAN)</b>:
        nhập số đọc trên bảng màu, không nhập số NH₃ tra từ bảng pH của kit. NH₃ tự do được app tự tính từ amonia tổng + pH và nhiệt độ ESP.
        Chỉ cần nhập chỉ số có kit, ô khác để trống.
        Hồ RAS nên đo nitrite và KH vài ngày một lần.
      </p>

      <form className="advice-profile" onSubmit={handleSave}>
        {FIELDS.map((f) => (
          <label key={f.key}>
            {f.label}
            <input
              type="number"
              min="0"
              max={f.max}
              step={f.step}
              placeholder={f.placeholder}
              value={form[f.key]}
              onChange={(e) => setForm((prev) => ({ ...prev, [f.key]: e.target.value }))}
            />
          </label>
        ))}
        <label>
          Thời điểm đo
          <input
            type="datetime-local"
            value={form.measured_at}
            onChange={(e) => setForm((f) => ({ ...f, measured_at: e.target.value }))}
          />
          <span className="advice-field-hint">Để trống = lúc lưu</span>
        </label>
        <button className="btn btn-primary" type="submit" disabled={saving || !deviceId}>
          {saving ? 'Đang lưu…' : 'Lưu vào lịch sử'}
        </button>
      </form>

      {message && <p className="advice-field-hint" style={{ marginBottom: 12 }}>{message}</p>}

      <div className="table-responsive">
        <table>
          <thead>
            <tr>
              <th>Thời điểm đo</th>
              <th>DO (mg/L)</th>
              <th>NH₄/NH₃ tổng (mg/L)</th>
              <th>NH₃ tự do (mg/L)</th>
              <th>NO₂ (mg/L)</th>
              <th>NO₃ (mg/L)</th>
              <th>KH (dKH)</th>
              <th>pH lúc tính</th>
              <th>Nguồn</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr>
                <td colSpan="9" style={{ textAlign: 'center', color: 'var(--text-dim)', padding: '24px' }}>
                  Đang tải…
                </td>
              </tr>
            ) : rows.length > 0 ? (
              rows.map((r) => (
                <tr key={r.id}>
                  <td style={{ color: 'var(--text-muted)' }}>
                    {r.measured_at ? new Date(r.measured_at).toLocaleString('vi-VN') : '—'}
                  </td>
                  <td style={{ color: '#38bdf8', fontWeight: 600 }}>{fmt(r.do_mg_l, 1)}</td>
                  <td style={{ color: '#fbbf24', fontWeight: 600 }}>{fmt(r.tan_mg_l, 2)}</td>
                  <td style={{ color: '#f87171', fontWeight: 600 }}>{fmt(r.nh3_free_mg_l, 3)}</td>
                  <td style={{ color: '#fb923c', fontWeight: 600 }}>{fmt(r.no2_mg_l, 2)}</td>
                  <td style={{ color: '#a78bfa', fontWeight: 600 }}>{fmt(r.no3_mg_l, 0)}</td>
                  <td style={{ color: '#34d399', fontWeight: 600 }}>{fmt(r.kh_dkh, 1)}</td>
                  <td>{fmt(r.ph_used, 2)}</td>
                  <td>
                    <span
                      style={{
                        fontSize: '11px',
                        padding: '2px 8px',
                        borderRadius: '10px',
                        background: 'rgba(56, 189, 248, 0.12)',
                        color: 'var(--accent)',
                      }}
                    >
                      {r.source || 'kit'}
                    </span>
                  </td>
                </tr>
              ))
            ) : (
              <tr>
                <td colSpan="9" style={{ textAlign: 'center', color: 'var(--text-dim)', padding: '30px' }}>
                  Chưa có mẫu kit. Nhập chỉ số rồi lưu.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
