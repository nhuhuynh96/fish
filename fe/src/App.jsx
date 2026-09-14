import React, { useState, useEffect, useCallback } from 'react';
import { api, createWebSocket } from './services/api';
import Header from './components/Header';
import SensorCards from './components/SensorCards';
import LiveSamplingTimeline from './components/LiveSamplingTimeline';
import SchedulePanel from './components/SchedulePanel';
import ControlPanel from './components/ControlPanel';
import EventLogStream from './components/EventLogStream';
import HistoryTable from './components/HistoryTable';
import CalibrationPanel from './components/CalibrationPanel';

function mergeMeasurement(prev, incoming) {
  if (!prev) return incoming;
  if (!incoming) return prev;
  const merged = { ...prev, ...incoming };
  const fields = [
    'temperature', 'ph', 'turbidity', 'tds',
    'ph_adc', 'ph_voltage', 'tds_adc', 'tds_voltage', 'turbidity_adc', 'turbidity_voltage',
  ];
  for (const f of fields) {
    if (incoming[f] === undefined || incoming[f] === null) {
      merged[f] = prev[f];
    }
  }
  return merged;
}

export default function App() {
  const [devices, setDevices] = useState([]);
  const [selectedDevice, setSelectedDevice] = useState('esp32_6ecb08');
  const [wsConnected, setWsConnected] = useState(false);
  const [latestMeasurement, setLatestMeasurement] = useState(null);
  const [history, setHistory] = useState([]);
  const [events, setEvents] = useState([]);
  const [currentState, setCurrentState] = useState('IDLE');
  const [deviceStatus, setDeviceStatus] = useState({ online: true });

  const loadData = useCallback(async (devId) => {
    try {
      const devs = await api.getDevices();
      setDevices(devs);

      let activeId = devId;
      if (devs && devs.length > 0) {
        const found = devs.find((d) => d.id === devId);
        if (!found) {
          activeId = devs[0].id;
          setSelectedDevice(activeId);
        }
      }

      const latest = await api.getLatest(activeId);
      if (latest) setLatestMeasurement(latest);

      const hist = await api.getHistory(activeId, 15);
      setHistory(hist);

      const evts = await api.getEvents(activeId, 25);
      setEvents(evts);
    } catch (e) {
      console.error('Load data error:', e);
    }
  }, []);

  useEffect(() => {
    loadData(selectedDevice);
  }, [selectedDevice, loadData]);

  // Thiết lập kết nối WebSocket Realtime
  useEffect(() => {
    const cleanup = createWebSocket(
      (msg) => {
        const { type, payload } = msg;

        if (type === 'sensor_data') {
          if (payload.device_id === selectedDevice) {
            setLatestMeasurement((prev) => mergeMeasurement(prev, payload));
            setHistory((prev) => [payload, ...prev.slice(0, 19)]);
            setCurrentState('IDLE');
          }
        } else if (type === 'sampling_event') {
          if (payload.device_id === selectedDevice) {
            setEvents((prev) => [payload, ...prev.slice(0, 29)]);
            if (payload.state) {
              setCurrentState(payload.state);
            }
          }
        } else if (type === 'device_status') {
          setDevices((prev) => {
            const exists = prev.find((d) => d.id === payload.id);
            if (!exists) return [...prev, payload];
            return prev.map((d) => (d.id === payload.id ? { ...d, ...payload } : d));
          });
          if (payload.id === selectedDevice) {
            setDeviceStatus(payload);
            if (payload.state) {
              setCurrentState(payload.state);
            }
          }
        }
      },
      (connected) => setWsConnected(connected)
    );

    return cleanup;
  }, [selectedDevice]);

  const handleMeasure = async (sensors) => {
    try {
      await api.triggerMeasure(selectedDevice, sensors);
    } catch (e) {
      console.error('Trigger measure error:', e);
      alert('Không thể gửi lệnh đo tới Server: ' + e.message);
    }
  };

  const handlePump = async (target, state) => {
    try {
      await api.setPump(selectedDevice, target, state);
    } catch (e) {
      console.error('Pump error:', e);
    }
  };

  const handleSetSchedule = async (scheduleData) => {
    try {
      return await api.setSchedule(selectedDevice, scheduleData);
    } catch (e) {
      console.error('Set schedule error:', e);
      throw e;
    }
  };

  const handleToggleAuto = async (enabled) => {
    try {
      return await api.toggleAuto(selectedDevice, enabled);
    } catch (e) {
      console.error('Toggle auto error:', e);
      throw e;
    }
  };

  return (
    <div className="app-container">
      <Header
        devices={devices}
        selectedDevice={selectedDevice}
        setSelectedDevice={setSelectedDevice}
        wsConnected={wsConnected}
        deviceStatus={deviceStatus}
        currentState={currentState}
      />

      <SensorCards measurement={latestMeasurement} />

      <LiveSamplingTimeline
        currentState={currentState}
        latestEvent={events.length > 0 ? events[0] : null}
      />

      {/* Main Grid: Control Panel, Auto-Schedule, Event Stream */}
      <div className="main-sections-grid" style={{ marginBottom: '24px' }}>
        <ControlPanel
          onMeasure={handleMeasure}
          onPump={handlePump}
          isMeasuring={currentState !== 'IDLE'}
        />
        <EventLogStream events={events} />
      </div>

      <div style={{ marginBottom: '28px' }}>
        <SchedulePanel
          deviceId={selectedDevice}
          onSetSchedule={handleSetSchedule}
          onToggleAuto={handleToggleAuto}
        />
      </div>

      <div style={{ marginBottom: '28px' }}>
        <CalibrationPanel deviceId={selectedDevice} latestMeasurement={latestMeasurement} />
      </div>

      <HistoryTable history={history} />
    </div>
  );
}
