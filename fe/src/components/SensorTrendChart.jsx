import React, { useMemo } from 'react';
import {
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
} from 'recharts';

const SERIES = [
  { key: 'ph', name: 'pH', color: '#34d399', unit: '', domain: [0, 14], digits: 2 },
  { key: 'tds', name: 'TDS', color: '#818cf8', unit: 'ppm', domain: ['auto', 'auto'], digits: 0 },
  { key: 'turbidity', name: 'Độ đục', color: '#38bdf8', unit: 'NTU', domain: ['auto', 'auto'], digits: 1 },
];

function formatTick(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString('vi-VN', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function buildChartRows(history) {
  if (!history?.length) return [];

  const chronological = [...history].reverse();
  const last = { ph: null, tds: null, turbidity: null };
  const rows = [];

  for (const m of chronological) {
    const t = m.created_at || null;
    if (m.ph != null) last.ph = m.ph;
    if (m.tds != null) last.tds = m.tds;
    if (m.turbidity != null) last.turbidity = m.turbidity;

    const hasAny = m.ph != null || m.tds != null || m.turbidity != null;
    if (!hasAny) continue;

    rows.push({
      t,
      label: formatTick(t),
      ph: last.ph,
      tds: last.tds,
      turbidity: last.turbidity,
    });
  }

  return rows;
}

function ChartTooltip({ active, payload, label }) {
  if (!active || !payload?.length) return null;
  return (
    <div className="chart-tooltip">
      <div className="chart-tooltip-time">{label}</div>
      {payload.map((p) => {
        const meta = SERIES.find((s) => s.key === p.dataKey);
        if (p.value == null) return null;
        const digits = meta?.digits ?? 1;
        const unit = meta?.unit ? ` ${meta.unit}` : '';
        return (
          <div key={p.dataKey} style={{ color: p.color }}>
            {meta?.name || p.name}: {Number(p.value).toFixed(digits)}{unit}
          </div>
        );
      })}
    </div>
  );
}

function MiniChart({ data, series }) {
  const hasData = data.some((d) => d[series.key] != null);
  return (
    <div className="trend-chart-panel">
      <div className="trend-chart-label" style={{ color: series.color }}>
        {series.name}
        {series.unit ? ` (${series.unit})` : ''}
      </div>
      {!hasData ? (
        <div className="trend-chart-empty">Chưa có dữ liệu {series.name}</div>
      ) : (
        <ResponsiveContainer width="100%" height={220}>
          <LineChart data={data} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
            <CartesianGrid stroke="rgba(255,255,255,0.06)" strokeDasharray="3 3" />
            <XAxis
              dataKey="label"
              tick={{ fill: '#64748b', fontSize: 10 }}
              minTickGap={28}
              axisLine={{ stroke: 'rgba(255,255,255,0.1)' }}
              tickLine={false}
            />
            <YAxis
              domain={series.domain}
              tick={{ fill: '#64748b', fontSize: 10 }}
              width={42}
              axisLine={false}
              tickLine={false}
            />
            <Tooltip content={<ChartTooltip />} />
            <Line
              type="monotone"
              dataKey={series.key}
              name={series.name}
              stroke={series.color}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4 }}
              connectNulls
              isAnimationActive={false}
            />
          </LineChart>
        </ResponsiveContainer>
      )}
    </div>
  );
}

export default function SensorTrendChart({ history }) {
  const rows = useMemo(() => buildChartRows(history), [history]);

  return (
    <div className="glass-card trend-chart-card">
      <div className="section-title">
        <svg width="18" height="18" fill="var(--primary)" viewBox="0 0 24 24">
          <path d="M3.5 18.49l6-6.01 4 4L22 6.92l-1.41-1.41-7.09 7.97-4-4L2 16.99z" />
        </svg>
        Biến Thiên Chỉ Số
        <span className="trend-chart-meta">
          {rows.length > 0 ? `${rows.length} điểm (pH · TDS · độ đục)` : 'Chưa có dữ liệu'}
        </span>
      </div>

      {rows.length === 0 ? (
        <p style={{ color: 'var(--text-dim)', fontSize: '0.9rem', padding: '24px 0', textAlign: 'center' }}>
          Chưa có lịch sử đo. Bấm Measure hoặc bật lịch tự động để thấy biểu đồ.
        </p>
      ) : (
        <div className="trend-chart-grid">
          {SERIES.map((s) => (
            <MiniChart key={s.key} data={rows} series={s} />
          ))}
        </div>
      )}
    </div>
  );
}
