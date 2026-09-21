import React from 'react';

export default function LiveSamplingTimeline({ currentState, latestEvent }) {
  const steps = [
    { key: 'FILLING_WATER', label: '1. Bơm Nạp', desc: 'Đầu queue: bơm đến phao đầy' },
    { key: 'STABILIZING', label: '2. Đo Cảm Biến', desc: 'Đọc ADC từng sensor' },
    { key: 'DRAINING_WATER', label: '3. Xả Nước', desc: 'Cuối queue: xả đến phao cạn' },
  ];

  const getStepStatus = (stepKey, index) => {
    if (!currentState || currentState === 'IDLE') return '';
    const stateOrder = ['FILLING_WATER', 'STABILIZING', 'DRAINING_WATER'];
    let mapped = currentState;
    if (currentState === 'MEASURING' || currentState === 'PUBLISHING') mapped = 'STABILIZING';
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
        Measure xếp queue: bơm nạp (đầu) → đo cảm biến → xả (cuối). Phao đầy/cạn hoặc tối đa 60s.
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
