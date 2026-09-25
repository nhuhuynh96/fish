import React from 'react';

const EEL_FALLBACK = {
  thresholds: {
    temp_min: 26, temp_max: 32,
    ph_min: 7.0, ph_max: 8.5,
    tds_min: 100, tds_max: 800,
  },
  badges: {
    temperature: { ok: 'Lý tưởng chình (26–32°C)', low: 'Hơi lạnh (<26°C)', high: 'Nóng (>32°C)' },
    ph: { ok: 'Chuẩn hồ chình (7.0–8.5)', low: 'Nhiễm axit (<7.0)', high: 'Nhiễm kiềm (>8.5)' },
    tds: { ok: 'Khoáng hồ chình (100–800 ppm)', low: 'Khoáng thấp (<100 ppm)', high: 'Khoáng/TDS cao (>800 ppm)' },
  },
};

function waiting() {
  return { text: 'Chờ lấy mẫu', cls: 'badge-ideal' };
}

export default function SensorCards({ measurement, pondConfig }) {
  const th = pondConfig?.thresholds || EEL_FALLBACK.thresholds;
  const badges = pondConfig?.badges || EEL_FALLBACK.badges;

  const temp = measurement?.temperature;
  const ph = measurement?.ph;
  const tds = measurement?.tds;

  const getTempStatus = (val) => {
    if (val === undefined || val === null) return waiting();
    if (val >= th.temp_min && val <= th.temp_max) return { text: badges.temperature.ok, cls: 'badge-ideal' };
    if (val < th.temp_min) return { text: badges.temperature.low, cls: 'badge-warning' };
    return { text: badges.temperature.high, cls: 'badge-danger' };
  };

  const getPhStatus = (val) => {
    if (val === undefined || val === null) return waiting();
    if (val >= th.ph_min && val <= th.ph_max) return { text: badges.ph.ok, cls: 'badge-ideal' };
    if (val < th.ph_min) return { text: badges.ph.low, cls: 'badge-danger' };
    return { text: badges.ph.high, cls: 'badge-warning' };
  };

  const getTdsStatus = (val) => {
    if (val === undefined || val === null) return waiting();
    if (val >= th.tds_min && val <= th.tds_max) return { text: badges.tds.ok, cls: 'badge-ideal' };
    if (val < th.tds_min) return { text: badges.tds.low, cls: 'badge-warning' };
    return { text: badges.tds.high, cls: 'badge-danger' };
  };

  const cards = [
    {
      title: 'Nhiệt Độ Nước',
      value: temp !== undefined && temp !== null ? temp.toFixed(1) : '--',
      unit: '°C',
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
      title: 'Chất Rắn Hòa Tan (TDS)',
      value: tds !== undefined && tds !== null ? Math.round(tds) : '--',
      unit: 'ppm',
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
          <div className={`sensor-badge ${c.status.cls}`}>{c.status.text}</div>
        </div>
      ))}
    </div>
  );
}
