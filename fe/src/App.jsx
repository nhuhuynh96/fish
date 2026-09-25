import React, { useState, useEffect, useCallback } from 'react';
import { api, createWebSocket } from './services/api';
import Header from './components/Header';
import NavMenu from './components/NavMenu';
import SensorCards from './components/SensorCards';
import AdvicePanel from './components/AdvicePanel';
import LiveSamplingTimeline from './components/LiveSamplingTimeline';
import SchedulePanel from './components/SchedulePanel';
import ControlPanel from './components/ControlPanel';
import EventLogStream from './components/EventLogStream';
import HistoryTable from './components/HistoryTable';
import SensorTrendChart from './components/SensorTrendChart';
import CalibrationPanel from './components/CalibrationPanel';
import KitLogPanel from './components/KitLogPanel';
import DeviceLogStream from './components/DeviceLogStream';

const HISTORY_LIMIT = 100;

function mergeMeasurement(prev, incoming) {
  if (!prev) return incoming;
  if (!incoming) return prev;
  const merged = { ...prev, ...incoming };
  const fields = [
    'temperature', 'ph', 'tds',
    'ph_adc', 'ph_voltage', 'tds_adc', 'tds_voltage',
  ];
  for (const f of fields) {
    if (incoming[f] === undefined || incoming[f] === null) {
      merged[f] = prev[f];
    }
  }
  return merged;
}

function applyPumpFromEvent(payload, setInletOn, setDrainOn) {
  if (typeof payload.inlet_on === 'boolean') {
    setInletOn(payload.inlet_on);
  } else {
    const stage = payload.stage || '';
    const msg = payload.message || '';
    if (msg.includes('Bơm nạp: BẬT') || msg.includes('Bơm nạp đã bật')) setInletOn(true);
    if (
      stage === 'queue_cleared' ||
      stage === 'manual_pump_timeout' ||
      msg.includes('Đã tắt bơm nạp') ||
      msg.includes('Tắt bơm') ||
      msg.includes('Không bơm nữa') ||
      msg.includes('Đã ngưng')
    ) {
      setInletOn(false);
    }
  }

  if (typeof payload.drain_on === 'boolean') {
    setDrainOn(payload.drain_on);
  } else {
    const stage = payload.stage || '';
    const msg = payload.message || '';
    if (stage === 'draining' || msg.includes('Van xả: BẬT')) setDrainOn(true);
    if (
      stage === 'drained' ||
      stage === 'queue_cleared' ||
      msg.includes('Đã tắt van xả') ||
      msg.includes('Đã xả 30s')
    ) {
      setDrainOn(false);
    }
  }
}

export default function App() {
  const [activeTab, setActiveTab] = useState('dashboard');
  const [devices, setDevices] = useState([]);
  const [selectedDevice, setSelectedDevice] = useState('esp32_4b70');
  const [wsConnected, setWsConnected] = useState(false);
  const [latestMeasurement, setLatestMeasurement] = useState(null);
  const [history, setHistory] = useState([]);
  const [events, setEvents] = useState([]);
  const [deviceLogs, setDeviceLogs] = useState([]);
  const [currentState, setCurrentState] = useState('IDLE');
  const [deviceStatus, setDeviceStatus] = useState({ online: true });
  const [pondConfig, setPondConfig] = useState(null);
  const [inletOn, setInletOn] = useState(false);
  const [drainOn, setDrainOn] = useState(false);

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

      const hist = await api.getHistory(activeId, HISTORY_LIMIT);
      setHistory(hist);

      const evts = await api.getEvents(activeId, 25);
      setEvents(evts);

      try {
        const pond = await api.getPondConfig();
        if (pond) setPondConfig(pond);
      } catch (e) {
        console.error('Load pond config error:', e);
      }
    } catch (e) {
      console.error('Load data error:', e);
    }
  }, []);

  useEffect(() => {
    loadData(selectedDevice);
    setDeviceLogs([]);
    setInletOn(false);
    setDrainOn(false);
  }, [selectedDevice, loadData]);

  useEffect(() => {
    const cleanup = createWebSocket(
      (msg) => {
        const { type, payload } = msg;

        if (type === 'sensor_data') {
          if (payload.device_id === selectedDevice) {
            setLatestMeasurement((prev) => mergeMeasurement(prev, payload));
            setHistory((prev) => [payload, ...prev.slice(0, HISTORY_LIMIT - 1)]);
            setCurrentState('IDLE');
          }
        } else if (type === 'sampling_event') {
          if (payload.device_id === selectedDevice) {
            setEvents((prev) => [payload, ...prev.slice(0, 29)]);
            if (payload.state) {
              setCurrentState(payload.state);
            }
            applyPumpFromEvent(payload, setInletOn, setDrainOn);
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
        } else if (type === 'pond_config_updated') {
          setPondConfig(payload);
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

  const handlePump = async (target, state, level = 2) => {
    if (target === 'inlet') setInletOn(state);
    if (target === 'drain') setDrainOn(state);
    try {
      await api.setPump(selectedDevice, target, state, level);
    } catch (e) {
      if (target === 'inlet') setInletOn(!state);
      if (target === 'drain') setDrainOn(!state);
      console.error('Pump error:', e);
    }
  };

  const handleClearQueue = async () => {
    setInletOn(false);
    setDrainOn(false);
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
          <SensorCards measurement={latestMeasurement} pondConfig={pondConfig} />

          <AdvicePanel
            deviceId={selectedDevice}
            onPump={handlePump}
            pondConfig={pondConfig}
            onPondConfigChange={setPondConfig}
          />

          <SensorTrendChart history={history} />

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
              inletOn={inletOn}
              drainOn={drainOn}
            />
            <EventLogStream events={events} />
          </div>

          <DeviceLogStream logs={deviceLogs} />
        </section>
      )}

      {activeTab === 'kit' && (
        <section className="page-section">
          <KitLogPanel deviceId={selectedDevice} />
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
          <SensorTrendChart history={history} />
          <HistoryTable history={history} />
        </section>
      )}
    </div>
  );
}
