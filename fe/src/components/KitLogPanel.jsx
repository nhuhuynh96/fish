import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../services/api';

function fmt(v, digits) {
  if (v == null) return '—';
  return Number(v).toFixed(digits);
}

export default function KitLogPanel({ deviceId }) {
  const [form, setForm] = useState({ do_mg_l: '', tan_mg_l: '', measured_at: '' });
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
    setForm({ do_mg_l: '', tan_mg_l: '', measured_at: '' });
    setMessage('');
    load();
  }, [deviceId, load]);

  const handleSave = async (e) => {
    e.preventDefault();
    if (!deviceId) return;
    const doVal = form.do_mg_l === '' ? null : Number(form.do_mg_l);
    const tanVal = form.tan_mg_l === '' ? null : Number(form.tan_mg_l);
    if ((doVal == null || Number.isNaN(doVal)) && (tanVal == null || Number.isNaN(tanVal))) {
      setMessage('Nhập ít nhất Oxy hoặc Amonia.');
      return;
    }
    setSaving(true);
    setMessage('');
    try {
      let measured_at;
      if (form.measured_at) {
        measured_at = new Date(form.measured_at).toISOString();
      }
      await api.saveKitReading(deviceId, {
        do_mg_l: doVal == null || Number.isNaN(doVal) ? null : doVal,
        tan_mg_l: tanVal == null || Number.isNaN(tanVal) ? null : tanVal,
        measured_at,
      });
      setForm({ do_mg_l: '', tan_mg_l: '', measured_at: '' });
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
        Nhật ký test kit (Oxy / Amonia)
      </div>

      <p className="advice-lead">
        Số từ que kit, không phải cảm biến ESP. Que thường in <b>TAN (NH₃+NH₄) mg/L</b>.
        NH₃ tự do được ước lượng từ TAN + pH ESP. Phân tích xu hướng tự lấy mẫu mới nhất (&lt; 24 giờ).
      </p>

      <form className="advice-profile" onSubmit={handleSave}>
        <label>
          Oxy hòa tan DO (mg/L)
          <input
            type="number"
            min="0"
            max="20"
            step="0.1"
            placeholder="vd. 6.5"
            value={form.do_mg_l}
            onChange={(e) => setForm((f) => ({ ...f, do_mg_l: e.target.value }))}
          />
        </label>
        <label>
          Amonia TAN (mg/L)
          <input
            type="number"
            min="0"
            max="20"
            step="0.01"
            placeholder="vd. 0.25"
            value={form.tan_mg_l}
            onChange={(e) => setForm((f) => ({ ...f, tan_mg_l: e.target.value }))}
          />
        </label>
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
              <th>TAN (mg/L)</th>
              <th>NH₃ tự do (mg/L)</th>
              <th>pH lúc tính</th>
              <th>Nguồn</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr>
                <td colSpan="6" style={{ textAlign: 'center', color: 'var(--text-dim)', padding: '24px' }}>
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
                <td colSpan="6" style={{ textAlign: 'center', color: 'var(--text-dim)', padding: '30px' }}>
                  Chưa có mẫu kit. Nhập DO / TAN rồi lưu.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
