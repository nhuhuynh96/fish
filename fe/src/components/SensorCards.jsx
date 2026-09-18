import React from 'react';

export default function SensorCards({ measurement }) {
  const temp = measurement?.temperature;
  const ph = measurement?.ph;
  const turb = measurement?.turbidity;
  const tds = measurement?.tds;

  const getTempStatus = (val) => {
    if (val === undefined || val === null) return { text: 'Chờ lấy mẫu', cls: 'badge-ideal' };
    if (val >= 25 && val <= 29) return { text: 'Lý tưởng (25-29°C)', cls: 'badge-ideal' };
    if (val < 25) return { text: 'Hơi lạnh (<25°C)', cls: 'badge-warning' };
    return { text: 'Nóng (>29°C)', cls: 'badge-danger' };
  };

  const getPhStatus = (val) => {
    if (val === undefined || val === null) return { text: 'Chờ lấy mẫu', cls: 'badge-ideal' };
    if (val >= 6.8 && val <= 7.8) return { text: 'Chuẩn hồ cá (6.8-7.8)', cls: 'badge-ideal' };
    if (val < 6.8) return { text: 'Nhiễm Axit (<6.8)', cls: 'badge-danger' };
    return { text: 'Nhiễm Kiềm (>7.8)', cls: 'badge-warning' };
  };

  const getTurbStatus = (val) => {
    if (val === undefined || val === null) return { text: 'Chờ lấy mẫu', cls: 'badge-ideal' };
    if (val < 15) return { text: 'Nước rất trong (<15 NTU)', cls: 'badge-ideal' };
    if (val <= 25) return { text: 'Hơi đục (15-25 NTU)', cls: 'badge-warning' };
    return { text: 'Nước đục cao (>25 NTU)', cls: 'badge-danger' };
  };

  const getTdsStatus = (val) => {
    if (val === undefined || val === null) return { text: 'Chờ lấy mẫu', cls: 'badge-ideal' };
    if (val >= 120 && val <= 260) return { text: 'Nước ngọt chuẩn (120-260 ppm)', cls: 'badge-ideal' };
    if (val < 120) return { text: 'Khoáng thấp (<120 ppm)', cls: 'badge-warning' };
    return { text: 'Chất rắn cao (>260 ppm)', cls: 'badge-danger' };
  };

  const formatRaw = (adc, voltage) => {
    if (adc == null && voltage == null) return null;
    const parts = [];
    if (adc != null) parts.push(`ADC ${adc}`);
    if (voltage != null) parts.push(`${Number(voltage).toFixed(3)} V`);
    return parts.join(' · ');
  };

  const cards = [
    {
      title: 'Nhiệt Độ Nước',
      value: temp !== undefined && temp !== null ? temp.toFixed(1) : '--',
      unit: '°C',
      raw: null,
      status: getTempStatus(temp),
      iconBg: 'rgba(239, 68, 68, 0.15)',
      iconColor: '#f87171',
      gradient: 'linear-gradient(90deg, #f87171, #fb923c)',
      icon: (
        <svg width="20" height="20" fill="currentColor" viewBox="0 0 24 24">
          <path d="M15 13V5c0-1.66-1.34-3-3-3S9 3.34 9 5v8c-1.21.91-2 2.37-2 4 0 2.76 2.24 5 5 5s5-2.24 5-5c0-1.63-.79-3.09-2-4zm-4-8c0-.55.45-1 1-1s1 .45 1 1h-2z" />
        </svg>
      ),
    },
    {
      title: 'Độ pH Môi Trường',
      value: ph !== undefined && ph !== null ? ph.toFixed(2) : '--',
      unit: 'pH',
      raw: formatRaw(measurement?.ph_adc, measurement?.ph_voltage),
      status: getPhStatus(ph),
      iconBg: 'rgba(16, 185, 129, 0.15)',
      iconColor: '#34d399',
      gradient: 'linear-gradient(90deg, #34d399, #10b981)',
      icon: (
        <svg width="20" height="20" fill="currentColor" viewBox="0 0 24 24">
          <path d="M12 2.69l5.66 5.66a8 8 0 1 1-11.31 0z" />
        </svg>
      ),
    },
    {
      title: 'Độ Đục Của Nước',
      value: turb !== undefined && turb !== null ? turb.toFixed(1) : '--',
      unit: 'NTU',
      raw: formatRaw(measurement?.turbidity_adc, measurement?.turbidity_voltage),
      status: getTurbStatus(turb),
      iconBg: 'rgba(56, 189, 248, 0.15)',
      iconColor: '#38bdf8',
      gradient: 'linear-gradient(90deg, #38bdf8, #0284c7)',
      icon: (
        <svg width="20" height="20" fill="currentColor" viewBox="0 0 24 24">
          <path d="M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96z" />
        </svg>
      ),
    },
    {
      title: 'Chất Rắn Hòa Tan (TDS)',
      value: tds !== undefined && tds !== null ? Math.round(tds) : '--',
      unit: 'ppm',
      raw: formatRaw(measurement?.tds_adc, measurement?.tds_voltage),
      status: getTdsStatus(tds),
      iconBg: 'rgba(129, 140, 248, 0.15)',
      iconColor: '#818cf8',
      gradient: 'linear-gradient(90deg, #818cf8, #a855f7)',
      icon: (
        <svg width="20" height="20" fill="currentColor" viewBox="0 0 24 24">
          <path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-2 10h-4v4h-2v-4H7v-2h4V7h2v4h4v2z" />
        </svg>
      ),
    },
  ];

  return (
    <div className="dashboard-grid">
      {cards.map((c, i) => (
        <div key={i} className="glass-card sensor-card" style={{ '--accent-gradient': c.gradient }}>
          <div className="sensor-header">
            <div className="sensor-title">{c.title}</div>
            <div className="sensor-icon" style={{ background: c.iconBg, color: c.iconColor }}>
              {c.icon}
            </div>
          </div>
          <div className="sensor-val-wrap">
            <div className="sensor-value">{c.value}</div>
            <div className="sensor-unit">{c.unit}</div>
          </div>
          {c.raw && (
            <div style={{ fontSize: '11px', color: 'var(--text-dim)', fontFamily: 'var(--font-mono)', marginTop: '4px' }}>
              raw: {c.raw}
            </div>
          )}
          <div className={`sensor-badge ${c.status.cls}`}>{c.status.text}</div>
        </div>
      ))}
    </div>
  );
}
