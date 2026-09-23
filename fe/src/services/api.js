const API_BASE = '/api';
const WS_PROTOCOL = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const WS_BASE = `${WS_PROTOCOL}//${window.location.host}/ws`;

export const api = {
  async getDevices() {
    const res = await fetch(`${API_BASE}/devices`);
    const json = await res.json();
    return json.data || [];
  },

  async getLatest(deviceId) {
    if (!deviceId) return null;
    const res = await fetch(`${API_BASE}/devices/${deviceId}/latest`);
    const json = await res.json();
    return json.data || null;
  },

  async getHistory(deviceId, limit = 15) {
    if (!deviceId) return [];
    const res = await fetch(`${API_BASE}/devices/${deviceId}/history?limit=${limit}`);
    const json = await res.json();
    return json.data || [];
  },

  async getEvents(deviceId, limit = 20) {
    if (!deviceId) return [];
    const res = await fetch(`${API_BASE}/devices/${deviceId}/events?limit=${limit}`);
    const json = await res.json();
    return json.data || [];
  },

  async triggerMeasure(deviceId, sensors = ['ph', 'turbidity', 'tds']) {
    const res = await fetch(`${API_BASE}/devices/${deviceId}/measure`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ sensors }),
    });
    return res.json();
  },

  async setPump(deviceId, target, state, level = 2) {
    const res = await fetch(`${API_BASE}/devices/${deviceId}/pump`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ target, state, level }),
    });
    return res.json();
  },

  async clearQueue(deviceId) {
    const res = await fetch(`${API_BASE}/devices/${deviceId}/clear-queue`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
    return res.json();
  },

  async setSchedule(deviceId, { autoEnabled, tempInterval, phInterval, turbInterval, tdsInterval }) {
    const res = await fetch(`${API_BASE}/devices/${deviceId}/schedule`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        auto_enabled: autoEnabled,
        temp_interval: Number(tempInterval),
        ph_interval: Number(phInterval),
        turb_interval: Number(turbInterval),
        tds_interval: Number(tdsInterval),
      }),
    });
    return res.json();
  },

  async getSchedule(deviceId) {
    const res = await fetch(`${API_BASE}/devices/${deviceId}/schedule`);
    const json = await res.json();
    return json.data || null;
  },

  async toggleAuto(deviceId, enabled) {
    const res = await fetch(`${API_BASE}/devices/${deviceId}/auto`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled }),
    });
    return res.json();
  },

  async getCalibration(deviceId) {
    const res = await fetch(`${API_BASE}/devices/${deviceId}/calibration`);
    const json = await res.json();
    return json.data || null;
  },

  async updateCalibration(deviceId, calibration) {
    const res = await fetch(`${API_BASE}/devices/${deviceId}/calibration`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(calibration),
    });
    const json = await res.json();
    if (!res.ok) throw new Error(json.error || 'Update calibration failed');
    return json.data;
  },

  async getAdvice(deviceId, profile) {
    if (!deviceId) return null;
    const res = await fetch(`${API_BASE}/devices/${deviceId}/advice`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(profile || {}),
    });
    const json = await res.json();
    if (!res.ok) throw new Error(json.error || 'Advice failed');
    return json.data;
  },

  async saveKitReading(deviceId, { do_mg_l, tan_mg_l, measured_at }) {
    const res = await fetch(`${API_BASE}/devices/${deviceId}/kit-readings`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        do_mg_l: do_mg_l === '' || do_mg_l == null ? null : Number(do_mg_l),
        tan_mg_l: tan_mg_l === '' || tan_mg_l == null ? null : Number(tan_mg_l),
        measured_at: measured_at || undefined,
      }),
    });
    const json = await res.json();
    if (!res.ok) throw new Error(json.error || 'Lưu số kit thất bại');
    return json.data;
  },

  async getKitReadings(deviceId, limit = 10) {
    if (!deviceId) return [];
    const res = await fetch(`${API_BASE}/devices/${deviceId}/kit-readings?limit=${limit}`);
    const json = await res.json();
    if (!res.ok) throw new Error(json.error || 'Load kit failed');
    return json.data || [];
  },

  async getPondConfig() {
    const res = await fetch(`${API_BASE}/pond-config`);
    const json = await res.json();
    if (!res.ok) throw new Error(json.error || 'Load pond config failed');
    return json.data;
  },

  async updatePondConfig(config) {
    const res = await fetch(`${API_BASE}/pond-config`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config || {}),
    });
    const json = await res.json();
    if (!res.ok) throw new Error(json.error || 'Update pond config failed');
    return json.data;
  },
};

export function createWebSocket(onMessage, onStatusChange) {
  let ws = null;
  let reconnectTimer = null;

  function connect() {
    try {
      ws = new WebSocket(WS_BASE);

      ws.onopen = () => {
        console.log('[WS] Connected to backend');
        if (onStatusChange) onStatusChange(true);
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (onMessage) onMessage(data);
        } catch (e) {
          console.error('[WS] Parse error:', e);
        }
      };

      ws.onclose = () => {
        console.log('[WS] Disconnected, reconnecting in 3s...');
        if (onStatusChange) onStatusChange(false);
        clearTimeout(reconnectTimer);
        reconnectTimer = setTimeout(connect, 3000);
      };

      ws.onerror = (err) => {
        console.error('[WS] Error:', err);
        ws.close();
      };
    } catch (e) {
      console.error('[WS] Connect error:', e);
      clearTimeout(reconnectTimer);
      reconnectTimer = setTimeout(connect, 3000);
    }
  }

  connect();

  return () => {
    clearTimeout(reconnectTimer);
    if (ws) ws.close();
  };
}
