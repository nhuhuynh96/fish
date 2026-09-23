import React, { useState } from 'react';

export default function ControlPanel({
  onMeasure,
  onPump,
  onClearQueue,
  isMeasuring,
  inletOn = false,
  drainOn = false,
}) {
  const [selectedSensors, setSelectedSensors] = useState(['temp', 'ph']);
  const [inletLevel, setInletLevel] = useState(2);
  const [loading, setLoading] = useState(false);

  const availableSensors = [
    { id: 'temp', label: '🌡️ Nhiệt độ' },
    { id: 'ph', label: '🧪 Độ pH' },
    { id: 'turbidity', label: '🌊 Độ đục' },
    { id: 'tds', label: '💎 Chất rắn (TDS)' },
  ];

  const toggleSensor = (id) => {
    if (selectedSensors.includes(id)) {
      if (selectedSensors.length > 1) {
        setSelectedSensors(selectedSensors.filter((s) => s !== id));
      }
    } else {
      setSelectedSensors([...selectedSensors, id]);
    }
  };

  const handleMeasureAll = async () => {
    setLoading(true);
    await onMeasure(['ph', 'turbidity', 'tds']);
    setLoading(false);
  };

  const handleMeasureSelected = async () => {
    if (selectedSensors.length === 0) return;
    setLoading(true);
    await onMeasure(selectedSensors);
    setLoading(false);
  };

  const toggleInlet = async () => {
    await onPump('inlet', !inletOn, inletLevel);
  };

  const toggleDrain = async () => {
    await onPump('drain', !drainOn);
  };

  const handleClearQueue = async () => {
    if (!onClearQueue) return;
    setLoading(true);
    try {
      await onClearQueue();
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="glass-card">
      <div className="section-title">
        <svg width="18" height="18" fill="var(--primary)" viewBox="0 0 24 24">
          <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z" />
        </svg>
        Bảng Điều Khiển Lấy Mẫu & Đo Đạc
      </div>

      {/* Action 1: Measure All */}
      <div style={{ marginBottom: '20px' }}>
        <button
          className="btn btn-primary"
          style={{ width: '100%', justifyContent: 'center', fontSize: '15px', padding: '14px' }}
          onClick={handleMeasureAll}
          disabled={loading}
        >
          🚀 Đo Toàn Bộ Chỉ Số (ph + turbidity + tds)
        </button>
      </div>

      {/* Action 2: Selective Measurement */}
      <div style={{ marginBottom: '20px' }}>
        <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-muted)', marginBottom: '8px' }}>
          Đo Có Chọn Lọc (Chọn các chỉ số cần lấy):
        </div>
        <div className="sensor-toggle-group">
          {availableSensors.map((s) => (
            <button
              key={s.id}
              type="button"
              className={`toggle-pill ${selectedSensors.includes(s.id) ? 'selected' : ''}`}
              onClick={() => toggleSensor(s.id)}
            >
              {s.label}
            </button>
          ))}
        </div>
        <button
          className="btn btn-secondary"
          style={{ width: '100%', justifyContent: 'center' }}
          onClick={handleMeasureSelected}
          disabled={loading}
        >
          🧪 Kích Hoạt Đo Theo Mục Đã Chọn ({selectedSensors.length})
        </button>
        <button
          className="btn btn-secondary"
          style={{
            width: '100%',
            justifyContent: 'center',
            marginTop: '10px',
            borderColor: 'rgba(248, 113, 113, 0.45)',
            color: 'var(--danger)',
          }}
          onClick={handleClearQueue}
          disabled={loading}
        >
          🗑 Xóa hết hàng đợi (clear_queue)
        </button>
      </div>

      <div
        style={{
          fontSize: '11px',
          color: 'var(--text-dim)',
          background: 'rgba(15, 23, 42, 0.4)',
          padding: '8px 12px',
          borderRadius: '8px',
          marginBottom: '20px',
        }}
      >
        💡 MQTT <code>{'{"action":"inlet_on"|"inlet_off"|"drain_on"|"drain_off","level":1|2}'}</code>. Bơm nạp chọn phao 1 hoặc 2. Đo: ph/turb/tds.
      </div>

      {/* Action 3: Manual Pump Controls */}
      <div>
        <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-muted)', marginBottom: '8px' }}>
          Điều khiển bơm / xả (MQTT queue):
        </div>
        <div style={{ display: 'flex', gap: '8px', marginBottom: '8px' }}>
          <button
            type="button"
            className={`toggle-pill ${inletLevel === 1 ? 'selected' : ''}`}
            onClick={() => setInletLevel(1)}
          >
            Phao 1
          </button>
          <button
            type="button"
            className={`toggle-pill ${inletLevel === 2 ? 'selected' : ''}`}
            onClick={() => setInletLevel(2)}
          >
            Phao 2
          </button>
        </div>
        <div className="btn-group" style={{ margin: 0 }}>
          <button
            className={`btn ${inletOn ? 'btn-primary' : 'btn-secondary'}`}
            style={{ flex: 1, justifyContent: 'center' }}
            onClick={toggleInlet}
          >
            🚰 Bơm Nạp: {inletOn ? 'BẬT' : 'TẮT'}
          </button>
          <button
            className={`btn ${drainOn ? 'btn-primary' : 'btn-secondary'}`}
            style={{ flex: 1, justifyContent: 'center' }}
            onClick={toggleDrain}
          >
            💨 Bơm Xả: {drainOn ? 'BẬT' : 'TẮT'}
          </button>
        </div>
      </div>
    </div>
  );
}
