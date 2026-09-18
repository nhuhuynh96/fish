import React from 'react';

export default function LiveSamplingTimeline({ currentState, latestEvent }) {
  const steps = [
    { key: 'STABILIZING', label: '1. Chuẩn Bị', desc: 'Sẵn sàng đọc cảm biến' },
    { key: 'MEASURING', label: '2. Đo Cảm Biến', desc: 'Đọc ADC / voltage từng sensor' },
    { key: 'PUBLISHING', label: '3. Gửi MQTT', desc: 'Publish sensor_data lên backend' },
  ];

  const getStepStatus = (stepKey, index) => {
    if (!currentState || currentState === 'IDLE') return '';
    // FILLING/DRAINING cũ không còn trong chu trình measure
    const stateOrder = ['STABILIZING', 'MEASURING', 'PUBLISHING'];
    let mapped = currentState;
    if (currentState === 'FILLING_WATER') mapped = 'STABILIZING';
    if (currentState === 'DRAINING_WATER') mapped = 'PUBLISHING';
    const currentIndex = stateOrder.indexOf(mapped);

    if (currentIndex === -1) return '';
    if (currentIndex === index) return 'active';
    if (currentIndex > index) return 'completed';
    return '';
  };

  return (
    <div className="glass-card" style={{ marginBottom: '24px' }}>
      <div className="section-title">
        <svg width="18" height="18" fill="var(--primary)" viewBox="0 0 24 24">
          <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z" />
        </svg>
        Tiến Trình Đo Cảm Biến
      </div>
      <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '12px' }}>
        Measure không bơm/xả. Bơm nạp / van xả điều khiển bằng lệnh pump riêng.
      </p>

      <div className="timeline-stepper">
        {steps.map((s, idx) => {
          const status = getStepStatus(s.key, idx);
          return (
            <div key={s.key} className={`step-item ${status}`}>
              <div className="step-circle">
                {status === 'completed' ? '✓' : idx + 1}
              </div>
              <div className="step-label">{s.label}</div>
            </div>
          );
        })}
      </div>

      {latestEvent && (
        <div
          style={{
            marginTop: '16px',
            padding: '10px 16px',
            borderRadius: '12px',
            background: 'rgba(56, 189, 248, 0.08)',
            border: '1px solid rgba(56, 189, 248, 0.2)',
            fontSize: '13px',
            display: 'flex',
            alignItems: 'center',
            gap: '10px',
          }}
        >
          <span style={{ color: 'var(--primary)', fontWeight: 700 }}>[Realtime Event]:</span>
          <span style={{ color: '#fff' }}>{latestEvent.message}</span>
        </div>
      )}
    </div>
  );
}
