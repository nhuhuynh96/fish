import React, { useState, useEffect } from 'react';
import { api } from '../services/api';

export default function SchedulePanel({ deviceId, onSetSchedule, onToggleAuto }) {
  const [autoEnabled, setAutoEnabled] = useState(false);
  const [tempInterval, setTempInterval] = useState(60);
  const [phInterval, setPhInterval] = useState(120);
  const [turbInterval, setTurbInterval] = useState(180);
  const [tdsInterval, setTdsInterval] = useState(300);
  const [saving, setSaving] = useState(false);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    async function load() {
      if (!deviceId) return;
      setLoading(true);
      try {
        const sched = await api.getSchedule(deviceId);
        if (!active || !sched) return;
        setAutoEnabled(!!sched.auto_enabled);
        if (sched.temp_interval > 0) setTempInterval(sched.temp_interval);
        if (sched.ph_interval > 0) setPhInterval(sched.ph_interval);
        if (sched.turb_interval > 0) setTurbInterval(sched.turb_interval);
        if (sched.tds_interval > 0) setTdsInterval(sched.tds_interval);
      } catch (e) {
        console.error('Load schedule error:', e);
      } finally {
        if (active) setLoading(false);
      }
    }
    load();
    return () => { active = false; };
  }, [deviceId]);

  const applyPreset = (temp, ph, turb, tds) => {
    setTempInterval(temp);
    setPhInterval(ph);
    setTurbInterval(turb);
    setTdsInterval(tds);
  };

  const handleSave = async (e) => {
    e.preventDefault();
    if (!deviceId) return;
    setSaving(true);
    try {
      await onSetSchedule({
        autoEnabled,
        tempInterval: Number(tempInterval),
        phInterval: Number(phInterval),
        turbInterval: Number(turbInterval),
        tdsInterval: Number(tdsInterval),
      });
      setSaveSuccess(true);
      setTimeout(() => setSaveSuccess(false), 3000);
    } catch (err) {
      console.error(err);
    } finally {
      setSaving(false);
    }
  };

  const handleToggle = async () => {
    const next = !autoEnabled;
    const prev = autoEnabled;
    setAutoEnabled(next);
    if (!deviceId) return;
    try {
      await onToggleAuto(next);
    } catch (err) {
      console.error(err);
      setAutoEnabled(prev);
    }
  };

  return (
    <div className="glass-card">
      <div className="section-title" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <svg width="18" height="18" fill="var(--primary)" viewBox="0 0 24 24">
            <path d="M11.99 2C6.47 2 2 6.48 2 12s4.47 10 9.99 10C17.52 22 22 17.52 22 12S17.52 2 11.99 2zM12 20c-4.42 0-8-3.58-8-8s3.58-8 8-8 8 3.58 8 8-3.58 8-8 8zm.5-13H11v6l5.25 3.15.75-1.23-4.5-2.67z" />
          </svg>
          Lịch Đo Tự Động Định Kỳ & Hàng Đợi
        </div>

        <button
          type="button"
          onClick={handleToggle}
          disabled={loading}
          style={{
            background: autoEnabled ? 'rgba(16, 185, 129, 0.2)' : 'rgba(239, 68, 68, 0.2)',
            border: `1px solid ${autoEnabled ? '#10b981' : '#ef4444'}`,
            color: autoEnabled ? '#10b981' : '#ef4444',
            padding: '4px 12px',
            borderRadius: '20px',
            cursor: loading ? 'wait' : 'pointer',
            fontSize: '12px',
            fontWeight: 700,
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
            transition: 'all 0.2s ease',
            opacity: loading ? 0.6 : 1,
          }}
        >
          <span
            style={{
              width: '8px',
              height: '8px',
              borderRadius: '50%',
              background: autoEnabled ? '#10b981' : '#ef4444',
              boxShadow: autoEnabled ? '0 0 8px #10b981' : 'none',
            }}
          />
          {loading ? 'AUTO: ...' : autoEnabled ? 'AUTO: ĐANG BẬT' : 'AUTO: ĐANG TẮT'}
        </button>
      </div>

      <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '14px' }}>
        Auto chạy trên <b>backend scheduler</b> (mỗi giây kiểm tra lịch → gửi MQTT measure). ESP32 không tự đếm giờ.
      </p>

      <div style={{ marginBottom: '16px' }}>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginBottom: '6px' }}>Cấu hình mẫu:</div>
        <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
          <button
            type="button"
            className="btn btn-secondary"
            style={{ padding: '6px 12px', fontSize: '12px' }}
            onClick={() => applyPreset(60, 120, 180, 300)}
          >
            ⏱️ Tiêu chuẩn (1p - 2p - 3p - 5p)
          </button>
          <button
            type="button"
            className="btn btn-secondary"
            style={{ padding: '6px 12px', fontSize: '12px' }}
            onClick={() => applyPreset(15, 30, 45, 60)}
          >
            ⚡ Test Nhanh (15s - 30s - 45s - 60s)
          </button>
          <button
            type="button"
            className="btn btn-secondary"
            style={{ padding: '6px 12px', fontSize: '12px' }}
            onClick={() => applyPreset(300, 600, 900, 1800)}
          >
            🔋 Tiết kiệm (5p - 10p - 15p - 30p)
          </button>
        </div>
      </div>

      <form onSubmit={handleSave}>
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))',
            gap: '12px',
            marginBottom: '16px',
          }}
        >
          <div className="form-group" style={{ margin: 0 }}>
            <label style={{ fontSize: '12px', color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }}>
              🌡️ Nhiệt độ (giây)
            </label>
            <input
              type="number"
              min="5"
              step="5"
              value={tempInterval}
              onChange={(e) => setTempInterval(e.target.value)}
              className="form-input"
              style={{
                width: '100%',
                padding: '8px 10px',
                borderRadius: '8px',
                background: 'rgba(15, 23, 42, 0.6)',
                border: '1px solid rgba(255, 255, 255, 0.1)',
                color: '#fff',
                fontSize: '13px',
              }}
            />
            <span style={{ fontSize: '10px', color: 'var(--text-dim)' }}>≈ {Math.round(tempInterval / 60 * 10) / 10} phút</span>
          </div>

          <div className="form-group" style={{ margin: 0 }}>
            <label style={{ fontSize: '12px', color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }}>
              🧪 Độ pH (giây)
            </label>
            <input
              type="number"
              min="5"
              step="5"
              value={phInterval}
              onChange={(e) => setPhInterval(e.target.value)}
              className="form-input"
              style={{
                width: '100%',
                padding: '8px 10px',
                borderRadius: '8px',
                background: 'rgba(15, 23, 42, 0.6)',
                border: '1px solid rgba(255, 255, 255, 0.1)',
                color: '#fff',
                fontSize: '13px',
              }}
            />
            <span style={{ fontSize: '10px', color: 'var(--text-dim)' }}>≈ {Math.round(phInterval / 60 * 10) / 10} phút</span>
          </div>

          <div className="form-group" style={{ margin: 0 }}>
            <label style={{ fontSize: '12px', color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }}>
              🌊 Độ đục (giây)
            </label>
            <input
              type="number"
              min="5"
              step="5"
              value={turbInterval}
              onChange={(e) => setTurbInterval(e.target.value)}
              className="form-input"
              style={{
                width: '100%',
                padding: '8px 10px',
                borderRadius: '8px',
                background: 'rgba(15, 23, 42, 0.6)',
                border: '1px solid rgba(255, 255, 255, 0.1)',
                color: '#fff',
                fontSize: '13px',
              }}
            />
            <span style={{ fontSize: '10px', color: 'var(--text-dim)' }}>≈ {Math.round(turbInterval / 60 * 10) / 10} phút</span>
          </div>

          <div className="form-group" style={{ margin: 0 }}>
            <label style={{ fontSize: '12px', color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }}>
              💎 TDS (giây)
            </label>
            <input
              type="number"
              min="5"
              step="5"
              value={tdsInterval}
              onChange={(e) => setTdsInterval(e.target.value)}
              className="form-input"
              style={{
                width: '100%',
                padding: '8px 10px',
                borderRadius: '8px',
                background: 'rgba(15, 23, 42, 0.6)',
                border: '1px solid rgba(255, 255, 255, 0.1)',
                color: '#fff',
                fontSize: '13px',
              }}
            />
            <span style={{ fontSize: '10px', color: 'var(--text-dim)' }}>≈ {Math.round(tdsInterval / 60 * 10) / 10} phút</span>
          </div>
        </div>

        <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
          <button
            type="submit"
            className="btn btn-primary"
            style={{ flex: 1, justifyContent: 'center' }}
            disabled={saving}
          >
            {saving ? '⏳ Đang lưu cấu hình...' : '💾 Cập Nhật Lịch Đo'}
          </button>
          {saveSuccess && (
            <span style={{ color: '#10b981', fontSize: '12px', fontWeight: 600 }}>
              ✓ Đã lưu thành công!
            </span>
          )}
        </div>
      </form>
    </div>
  );
}
