import React from 'react';

export default function HistoryTable({ history }) {
  return (
    <div className="glass-card">
      <div className="section-title">
        <svg width="18" height="18" fill="var(--primary)" viewBox="0 0 24 24">
          <path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-5 14H7v-2h7v2zm3-4H7v-2h10v2zm0-4H7V7h10v2z" />
        </svg>
        Lịch Sử Các Lần Lấy Mẫu Gần Đây
      </div>

      <div className="table-responsive">
        <table>
          <thead>
            <tr>
              <th>Thời Gian</th>
              <th>Nhiệt Độ (°C)</th>
              <th>Độ pH</th>
              <th>Độ Đục (NTU)</th>
              <th>TDS (ppm)</th>
              <th>Thời Gian Đo</th>
              <th>Trạng Thái</th>
            </tr>
          </thead>
          <tbody>
            {history && history.length > 0 ? (
              history.map((h) => {
                const timeStr = h.created_at
                  ? new Date(h.created_at).toLocaleString()
                  : '--';
                const duration = h.duration_ms
                  ? `${(h.duration_ms / 1000).toFixed(1)}s`
                  : '--';

                return (
                  <tr key={h.id || Math.random()}>
                    <td style={{ color: 'var(--text-muted)' }}>{timeStr}</td>
                    <td style={{ color: '#f87171', fontWeight: 600 }}>
                      {h.temperature !== undefined && h.temperature !== null
                        ? `${h.temperature.toFixed(1)} °C`
                        : '-'}
                    </td>
                    <td style={{ color: '#34d399', fontWeight: 600 }}>
                      {h.ph !== undefined && h.ph !== null ? h.ph.toFixed(2) : '-'}
                    </td>
                    <td style={{ color: '#38bdf8', fontWeight: 600 }}>
                      {h.turbidity !== undefined && h.turbidity !== null
                        ? `${h.turbidity.toFixed(1)} NTU`
                        : '-'}
                    </td>
                    <td style={{ color: '#818cf8', fontWeight: 600 }}>
                      {h.tds !== undefined && h.tds !== null
                        ? `${Math.round(h.tds)} ppm`
                        : '-'}
                    </td>
                    <td style={{ color: 'var(--text-dim)' }}>{duration}</td>
                    <td>
                      <span
                        style={{
                          fontSize: '11px',
                          padding: '2px 8px',
                          borderRadius: '10px',
                          background: 'rgba(52, 211, 153, 0.1)',
                          color: 'var(--success)',
                        }}
                      >
                        {h.status || 'Success'}
                      </span>
                    </td>
                  </tr>
                );
              })
            ) : (
              <tr>
                <td colSpan="7" style={{ textAlign: 'center', color: 'var(--text-dim)', padding: '30px' }}>
                  Chưa có dữ liệu lịch sử. Các lần lấy mẫu hoàn tất sẽ hiển thị tại đây.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
