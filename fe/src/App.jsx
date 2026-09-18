import React, { useState, useEffect, useCallback } from 'react';
import { api, createWebSocket } from './services/api';
import Header from './components/Header';
import NavMenu from './components/NavMenu';
import SensorCards from './components/SensorCards';
import LiveSamplingTimeline from './components/LiveSamplingTimeline';
import SchedulePanel from './components/SchedulePanel';
import ControlPanel from './components/ControlPanel';
import EventLogStream from './components/EventLogStream';
import HistoryTable from './components/HistoryTable';
import CalibrationPanel from './components/CalibrationPanel';
import DeviceLogStream from './components/DeviceLogStream';

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
  const [activeTab, setActiveTab] = useState('dashboard');
  const [devices, setDevices] = useState([]);
  const [selectedDevice, setSelectedDevice] = useState('esp32_6ecb08');
  const [wsConnected, setWsConnected] = useState(false);
  const [latestMeasurement, setLatestMeasurement] = useState(null);
  const [history, setHistory] = useState([]);
  const [events, setEvents] = useState([]);
  const [deviceLogs, setDeviceLogs] = useState([]);
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
    setDeviceLogs([]);
  }, [selectedDevice, loadData]);

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
        } else if (type === 'device_log') {
          if (payload.device_id === selectedDevice) {
            setDeviceLogs((prev) => [...prev.slice(-199), payload]);
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

  const handleClearQueue = async () => {
    try {
      await api.clearQueue(selectedDevice);
    } catch (e) {
      console.error('Clear queue error:', e);
      alert('Không thể gửi lệnh xóa queue: ' + e.message);
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

      <NavMenu activeTab={activeTab} onChange={setActiveTab} />

      {activeTab === 'dashboard' && (
        <section className="page-section">
          <SensorCards measurement={latestMeasurement} />

          <LiveSamplingTimeline
            currentState={currentState}
            latestEvent={events.length > 0 ? events[0] : null}
          />

          <div className="main-sections-grid" style={{ marginBottom: '24px' }}>
            <ControlPanel
              onMeasure={handleMeasure}
              onPump={handlePump}
              onClearQueue={handleClearQueue}
              isMeasuring={currentState !== 'IDLE'}
            />
            <EventLogStream events={events} />
          </div>

          <DeviceLogStream logs={deviceLogs} />
        </section>
      )}

      {activeTab === 'schedule' && (
        <section className="page-section">
          <SchedulePanel
            deviceId={selectedDevice}
            onSetSchedule={handleSetSchedule}
            onToggleAuto={handleToggleAuto}
          />
        </section>
      )}

      {activeTab === 'calibration' && (
        <section className="page-section">
          <CalibrationPanel
            deviceId={selectedDevice}
            latestMeasurement={latestMeasurement}
          />
        </section>
      )}

      {activeTab === 'history' && (
        <section className="page-section">
          <HistoryTable history={history} />
        </section>
      )}
    </div>
  );
}
