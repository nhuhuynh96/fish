import React from 'react';

const TABS = [
  { id: 'dashboard', label: 'Dashboard' },
  { id: 'schedule', label: 'Lịch Đo Tự Động' },
  { id: 'calibration', label: 'Hiệu Chuẩn' },
  { id: 'history', label: 'Lịch Sử' },
];

export default function NavMenu({ activeTab, onChange }) {
  return (
    <nav className="nav-menu" aria-label="Menu chính">
      {TABS.map((tab) => (
        <button
          key={tab.id}
          type="button"
          className={`nav-menu-item ${activeTab === tab.id ? 'active' : ''}`}
          onClick={() => onChange(tab.id)}
        >
          {tab.label}
        </button>
      ))}
    </nav>
  );
}
