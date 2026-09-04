import React from 'react';

export default function LiveSamplingTimeline({ currentState, latestEvent }) {
  const steps = [
    { key: 'FILLING_WATER', label: '1. Bơm Nạp Nước', desc: 'Bơm vào đến khi Phao ĐẦY' },
    { key: 'STABILIZING', label: '2. Lắng Nước', desc: 'Chờ nước ổn định 1s' },
    { key: 'MEASURING', label: '3. Đo Tuần Tự', desc: 'Cấp nguồn từng cảm biến (tránh nhiễu)' },
    { key: 'DRAINING_WATER', label: '4. Xả Nước', desc: 'Xả sạch buồng đo đến khi Phao CẠN' },
    { key: 'PUBLISHING', label: '5. Hoàn Tất', desc: 'Gửi kết quả đo về Server' },
  ];

  const getStepStatus = (stepKey, index) => {
    if (!currentState || currentState === 'IDLE') return '';
    const stateOrder = ['FILLING_WATER', 'STABILIZING', 'MEASURING', 'DRAINING_WATER', 'PUBLISHING'];
    const currentIndex = stateOrder.indexOf(currentState);

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
        Tiến Trình Chu Trình Lấy Mẫu (Sampling State Machine)
      </div>

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
