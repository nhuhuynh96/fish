import React, { useEffect, useState } from 'react';
import { api } from '../services/api';

const DEFAULTS = {
  ph_neutral_v: 2.5,
  ph_slope: 0.18,
  tds_temp_c: 25,
  tds_ref_v: 0,
  tds_ref_ppm: 0,
  tds_max_ppm: 2000,
  turb_v_clear: 2.15,
  turb_v_dirty: 1.0,
  turb_ntu_max: 1000,
};

export default function CalibrationPanel({ deviceId, latestMeasurement }) {
  const [form, setForm] = useState(DEFAULTS);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');

  useEffect(() => {
    let active = true;
    async function load() {
      setLoading(true);
      try {
        const cal = await api.getCalibration(deviceId);
        if (active && cal) {
          setForm({
            ph_neutral_v: cal.ph_neutral_v ?? DEFAULTS.ph_neutral_v,
            ph_slope: cal.ph_slope ?? DEFAULTS.ph_slope,
            tds_temp_c: cal.tds_temp_c ?? DEFAULTS.tds_temp_c,
            tds_ref_v: cal.tds_ref_v ?? DEFAULTS.tds_ref_v,
            tds_ref_ppm: cal.tds_ref_ppm ?? DEFAULTS.tds_ref_ppm,
            tds_max_ppm: cal.tds_max_ppm ?? DEFAULTS.tds_max_ppm,
            turb_v_clear: cal.turb_v_clear ?? DEFAULTS.turb_v_clear,
            turb_v_dirty: cal.turb_v_dirty ?? DEFAULTS.turb_v_dirty,
            turb_ntu_max: cal.turb_ntu_max ?? DEFAULTS.turb_ntu_max,
          });
        }
      } catch (e) {
        console.error('Load calibration error:', e);
      } finally {
        if (active) setLoading(false);
      }
    }
    if (deviceId) load();
    return () => { active = false; };
  }, [deviceId]);

  const handleChange = (field) => (e) => {
    setForm((prev) => ({ ...prev, [field]: parseFloat(e.target.value) || 0 }));
  };

  const handleSave = async () => {
    setSaving(true);
    setMessage('');
    try {
      await api.updateCalibration(deviceId, form);
      setMessage('Đã lưu hiệu chuẩn. Lần đo tiếp theo backend sẽ tính lại pH/TDS/NTU.');
    } catch (e) {
      setMessage('Lỗi: ' + e.message);
    } finally {
      setSaving(false);
    }
  };

  const applyPhVoltage = () => {
    if (latestMeasurement?.ph_voltage != null) {
      setForm((prev) => ({ ...prev, ph_neutral_v: latestMeasurement.ph_voltage }));
      setMessage(`Đã gán neutral_v = ${latestMeasurement.ph_voltage.toFixed(3)}V từ lần đo gần nhất (dùng khi ngâm buffer pH 7).`);
    }
  };

  const applyTdsVoltage = () => {
    if (latestMeasurement?.tds_voltage != null) {
      setForm((prev) => ({
        ...prev,
        tds_ref_v: latestMeasurement.tds_voltage,
        tds_ref_ppm: prev.tds_ref_ppm > 0 ? prev.tds_ref_ppm : 1382,
      }));
      setMessage(
        `Đã gán tds_ref_v = ${latestMeasurement.tds_voltage.toFixed(3)}V. Điền ppm trên chai rồi Lưu.`
      );
    }
  };

  return (
    <div className="glass-card">
      <div className="panel-header" style={{ marginBottom: '16px' }}>
        <h3 style={{ fontSize: '1.1rem', fontWeight: 600 }}>Hiệu Chuẩn Cảm Biến (Backend)</h3>
        <p style={{ color: 'var(--text-muted)', fontSize: '0.85rem', marginTop: '6px' }}>
          ESP32 gửi ADC + điện áp thô. Chỉnh tham số tại đây — không cần flash lại firmware.
        </p>
      </div>

      {loading ? (
        <p style={{ color: 'var(--text-muted)' }}>Đang tải cấu hình...</p>
      ) : (
        <div className="calibration-grid">
          <fieldset className="cal-fieldset">
            <legend>pH (PH-4502C)</legend>
            <label>Neutral voltage (V @ pH 7)
              <input type="number" step="0.001" value={form.ph_neutral_v} onChange={handleChange('ph_neutral_v')} />
            </label>
            <label>Slope (V/pH)
              <input type="number" step="0.001" value={form.ph_slope} onChange={handleChange('ph_slope')} />
            </label>
            <button type="button" className="btn-secondary" onClick={applyPhVoltage}>
              Lấy V từ lần đo pH gần nhất
            </button>
          </fieldset>

          <fieldset className="cal-fieldset">
            <legend>TDS (1 điểm chuẩn)</legend>
            <label>Nhiệt độ bù (°C)
              <input type="number" step="0.1" value={form.tds_temp_c} onChange={handleChange('tds_temp_c')} />
            </label>
            <label>V trong chuẩn (tds_ref_v)
              <input type="number" step="0.001" value={form.tds_ref_v} onChange={handleChange('tds_ref_v')} />
            </label>
            <label>PPM trên chai chuẩn
              <input type="number" step="1" value={form.tds_ref_ppm} onChange={handleChange('tds_ref_ppm')} />
            </label>
            <label>PPM max
              <input type="number" step="1" value={form.tds_max_ppm} onChange={handleChange('tds_max_ppm')} />
            </label>
            <button type="button" className="btn-secondary" onClick={applyTdsVoltage}>
              Lấy V từ lần đo TDS gần nhất
            </button>
            <p style={{ color: 'var(--text-dim)', fontSize: '0.75rem', marginTop: '8px' }}>
              Đặt ref_ppm = 0 để tắt scale (chỉ dùng công thức DFRobot).
            </p>
          </fieldset>
        </div>
      )}

      <div style={{ marginTop: '16px', display: 'flex', gap: '12px', alignItems: 'center' }}>
        <button type="button" className="btn-primary" onClick={handleSave} disabled={saving || loading}>
          {saving ? 'Đang lưu...' : 'Lưu hiệu chuẩn'}
        </button>
        {message && <span style={{ color: 'var(--text-muted)', fontSize: '0.85rem' }}>{message}</span>}
      </div>

      {latestMeasurement && (
        <div className="raw-debug" style={{ marginTop: '16px' }}>
          <div style={{ fontSize: '0.8rem', color: 'var(--text-dim)', marginBottom: '8px' }}>Raw gần nhất</div>
          <div className="raw-debug-grid">
            <span>pH: ADC {latestMeasurement.ph_adc ?? '--'} | {latestMeasurement.ph_voltage != null ? latestMeasurement.ph_voltage.toFixed(3) + 'V' : '--'}</span>
            <span>TDS: ADC {latestMeasurement.tds_adc ?? '--'} | {latestMeasurement.tds_voltage != null ? latestMeasurement.tds_voltage.toFixed(3) + 'V' : '--'}</span>
          </div>
        </div>
      )}
    </div>
  );
}
