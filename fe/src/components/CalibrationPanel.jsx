import React, { useEffect, useState } from 'react';
import { api } from '../services/api';

const PH_DEVICE = 'ph';

const TDS_DEFAULTS = {
  tds_temp_c: 25,
  tds_ref_v: 0,
  tds_ref_ppm: 0,
  tds_max_ppm: 2000,
};

const PH_DEFAULTS = { ph_gain: 1, ph_offset: 0 };

const BUFFERS = [4.01, 6.86, 9.18];

// Hồi quy tuyến tính pH chuẩn theo pH thô: chuẩn = gain * thô + offset.
function fitPh(points) {
  const n = points.length;
  if (n < 2) return null;
  const mx = points.reduce((s, p) => s + p.raw, 0) / n;
  const my = points.reduce((s, p) => s + p.ref, 0) / n;
  let sxy = 0;
  let sxx = 0;
  for (const p of points) {
    sxy += (p.raw - mx) * (p.ref - my);
    sxx += (p.raw - mx) ** 2;
  }
  if (sxx === 0) return null;
  const gain = sxy / sxx;
  return { gain, offset: my - gain * mx };
}

export default function CalibrationPanel({ deviceId, latestMeasurement }) {
  const [tds, setTds] = useState(TDS_DEFAULTS);
  const [ph, setPh] = useState(PH_DEFAULTS);
  const [rawReadings, setRawReadings] = useState({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');

  useEffect(() => {
    let active = true;
    async function load() {
      setLoading(true);
      try {
        // pH và TDS cùng ở bo ph nên dùng chung một bản hiệu chỉnh.
        const cal = await api.getCalibration(PH_DEVICE);
        const phCal = cal;
        if (!active) return;
        if (cal) {
          setTds({
            tds_temp_c: cal.tds_temp_c ?? TDS_DEFAULTS.tds_temp_c,
            tds_ref_v: cal.tds_ref_v ?? TDS_DEFAULTS.tds_ref_v,
            tds_ref_ppm: cal.tds_ref_ppm ?? TDS_DEFAULTS.tds_ref_ppm,
            tds_max_ppm: cal.tds_max_ppm ?? TDS_DEFAULTS.tds_max_ppm,
          });
        }
        if (phCal) {
          setPh({
            ph_gain: phCal.ph_gain || PH_DEFAULTS.ph_gain,
            ph_offset: phCal.ph_offset ?? PH_DEFAULTS.ph_offset,
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

  const handleTds = (field) => (e) => {
    setTds((prev) => ({ ...prev, [field]: parseFloat(e.target.value) || 0 }));
  };

  const handlePh = (field) => (e) => {
    setPh((prev) => ({ ...prev, [field]: parseFloat(e.target.value) || 0 }));
  };

  const handleRaw = (buffer) => (e) => {
    setRawReadings((prev) => ({ ...prev, [buffer]: e.target.value }));
  };

  const fillLatestRaw = (buffer) => {
    const raw = latestMeasurement?.ph_raw;
    if (raw == null) {
      setMessage('Chưa có pH thô từ bo pH. Bấm đo pH trước.');
      return;
    }
    setRawReadings((prev) => ({ ...prev, [buffer]: raw.toFixed(2) }));
  };

  const computePh = () => {
    const points = BUFFERS
      .filter((b) => rawReadings[b] !== undefined && rawReadings[b] !== '')
      .map((b) => ({ ref: b, raw: parseFloat(rawReadings[b]) }))
      .filter((p) => !Number.isNaN(p.raw));
    const fit = fitPh(points);
    if (!fit) {
      setMessage('Cần ít nhất 2 dung dịch chuẩn có pH thô khác nhau.');
      return;
    }
    if (fit.gain <= 0.5 || fit.gain >= 2 || Math.abs(fit.offset) >= 3) {
      setMessage(`Kết quả bất thường (gain ${fit.gain.toFixed(3)}, offset ${fit.offset.toFixed(3)}). Kiểm tra lại số đã nhập.`);
      return;
    }
    setPh({ ph_gain: Number(fit.gain.toFixed(4)), ph_offset: Number(fit.offset.toFixed(4)) });
    setMessage(`Tính được gain ${fit.gain.toFixed(4)}, offset ${fit.offset.toFixed(4)} từ ${points.length} điểm. Bấm Lưu để áp dụng.`);
  };

  const handleSave = async () => {
    setSaving(true);
    setMessage('');
    try {
      await api.updateCalibration(PH_DEVICE, { ...tds, ...ph });
      setMessage('Đã lưu hiệu chuẩn. Lần đo tiếp theo backend sẽ tính lại pH và TDS.');
    } catch (e) {
      setMessage('Lỗi: ' + e.message);
    } finally {
      setSaving(false);
    }
  };

  const applyTdsVoltage = () => {
    if (latestMeasurement?.tds_voltage != null) {
      setTds((prev) => ({
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
          Bo pH gửi pH thô của cảm biến RS485 và điện áp TDS (bù nhiệt theo nhiệt độ cảm biến pH). Chỉnh tham số tại đây, không cần nạp lại firmware.
        </p>
      </div>

      {loading ? (
        <p style={{ color: 'var(--text-muted)' }}>Đang tải cấu hình...</p>
      ) : (
        <div className="calibration-grid">
          <fieldset className="cal-fieldset">
            <legend>pH (RS485, bo ph)</legend>
            <p style={{ color: 'var(--text-dim)', fontSize: '0.75rem', marginBottom: '8px' }}>
              pH = gain × pH thô + offset. Ngâm đầu dò vào từng dung dịch chuẩn 1–2 phút, đo pH, rồi ghi pH thô.
            </p>
            {BUFFERS.map((b) => (
              <label key={b}>pH thô trong dung dịch {b}
                <div style={{ display: 'flex', gap: '6px' }}>
                  <input type="number" step="0.01" value={rawReadings[b] ?? ''} onChange={handleRaw(b)} placeholder="bỏ trống nếu không đo" />
                  <button type="button" className="btn-secondary" onClick={() => fillLatestRaw(b)}>Lấy</button>
                </div>
              </label>
            ))}
            <button type="button" className="btn-secondary" onClick={computePh}>
              Tính gain / offset
            </button>
            <label>Gain
              <input type="number" step="0.0001" value={ph.ph_gain} onChange={handlePh('ph_gain')} />
            </label>
            <label>Offset
              <input type="number" step="0.0001" value={ph.ph_offset} onChange={handlePh('ph_offset')} />
            </label>
          </fieldset>

          <fieldset className="cal-fieldset">
            <legend>TDS (1 điểm chuẩn)</legend>
            <label>Nhiệt độ bù khi không đọc được nhiệt độ cảm biến pH (°C)
              <input type="number" step="0.1" value={tds.tds_temp_c} onChange={handleTds('tds_temp_c')} />
            </label>
            <label>V trong chuẩn (tds_ref_v)
              <input type="number" step="0.001" value={tds.tds_ref_v} onChange={handleTds('tds_ref_v')} />
            </label>
            <label>PPM trên chai chuẩn
              <input type="number" step="1" value={tds.tds_ref_ppm} onChange={handleTds('tds_ref_ppm')} />
            </label>
            <label>PPM max
              <input type="number" step="1" value={tds.tds_max_ppm} onChange={handleTds('tds_max_ppm')} />
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
            <span>pH thô: {latestMeasurement.ph_raw != null ? latestMeasurement.ph_raw.toFixed(2) : '--'} | sau hiệu chỉnh: {latestMeasurement.ph != null ? latestMeasurement.ph.toFixed(2) : '--'}</span>
            <span>TDS: ADC {latestMeasurement.tds_adc ?? '--'} | {latestMeasurement.tds_voltage != null ? latestMeasurement.tds_voltage.toFixed(3) + 'V' : '--'}</span>
          </div>
        </div>
      )}
    </div>
  );
}
