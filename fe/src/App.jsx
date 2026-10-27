import React, { useState, useEffect, useCallback, useRef } from 'react';
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
const SENSOR_ID = 'sensor';
const WATER_ID = 'water';

function isBoard(id) {
  return id === SENSOR_ID || id === WATER_ID;
}

function mergeEvents(sensorEvents, waterEvents) {
  return [...(sensorEvents || []), ...(waterEvents || [])]
    .sort((a, b) => new Date(b.created_at || 0) - new Date(a.created_at || 0))
    .slice(0, 25);
}

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
  const [wsConnected, setWsConnected] = useState(false);
  const [latestMeasurement, setLatestMeasurement] = useState(null);
  const [history, setHistory] = useState([]);
  const [events, setEvents] = useState([]);
  const [deviceLogs, setDeviceLogs] = useState([]);
  const [currentState, setCurrentState] = useState('IDLE');
  const [jobPhase, setJobPhase] = useState(-1);
  const jobPhaseRef = useRef(-1);
  const setPhase = (phase) => {
    jobPhaseRef.current = phase;
    setJobPhase(phase);
  };
  const [pondConfig, setPondConfig] = useState(null);
  const [inletOn, setInletOn] = useState(false);
  const [drainOn, setDrainOn] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const devs = await api.getDevices();
      setDevices(devs);

      const latest = await api.getLatest(SENSOR_ID);
      if (latest) setLatestMeasurement(latest);

      const hist = await api.getHistory(SENSOR_ID, HISTORY_LIMIT);
      setHistory(hist);

      const [sensorEvents, waterEvents] = await Promise.all([
        api.getEvents(SENSOR_ID, 25),
        api.getEvents(WATER_ID, 25),
      ]);
      const merged = mergeEvents(sensorEvents, waterEvents);
      setEvents(merged);

      let inlet = false;
      let drain = false;
      [...merged].reverse().forEach((evt) => {
        if (evt.device_id === WATER_ID) {
          applyPumpFromEvent(evt, (v) => { inlet = v; }, (v) => { drain = v; });
        }
      });
      setInletOn(inlet);
      setDrainOn(drain);

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
    loadData();
  }, [loadData]);

  useEffect(() => {
    const cleanup = createWebSocket(
      (msg) => {
        const { type, payload } = msg;

        if (type === 'sensor_data') {
          if (payload.device_id === SENSOR_ID) {
            setLatestMeasurement((prev) => mergeMeasurement(prev, payload));
            setHistory((prev) => [payload, ...prev.slice(0, HISTORY_LIMIT - 1)]);
          }
        } else if (type === 'sampling_event') {
          if (isBoard(payload.device_id)) {
            setEvents((prev) => [payload, ...prev.slice(0, 29)]);
            const jobStages = ['fill', 'drain_level', 'settle', 'warmup', 'measure', 'final_drain'];
            if (payload.stage === 'fill' || payload.stage === 'drain_level' || payload.stage === 'settle') setPhase(0);
            else if (payload.stage === 'warmup' || payload.stage === 'measure') setPhase(1);
            else if (payload.stage === 'final_drain') setPhase(2);
            else if (payload.stage === 'job_done' || payload.stage === 'job_failed') {
              setPhase(-1);
              setCurrentState('IDLE');
            }
            if (jobStages.includes(payload.stage)) {
              setCurrentState('MEASURING');
            }
            if (payload.device_id === WATER_ID) {
              applyPumpFromEvent(payload, setInletOn, setDrainOn);
            }
          }
        } else if (type === 'device_status') {
          setDevices((prev) => {
            const exists = prev.find((d) => d.id === payload.id);
            if (!exists) return [...prev, payload];
            return prev.map((d) => (d.id === payload.id ? { ...d, ...payload } : d));
          });
          if (payload.id === SENSOR_ID && payload.state && jobPhaseRef.current < 0) {
            setCurrentState(payload.state);
          }
        } else if (type === 'pond_config_updated') {
          setPondConfig(payload);
        } else if (type === 'device_log') {
          if (isBoard(payload.device_id)) {
            setDeviceLogs((prev) => [...prev.slice(-199), payload]);
          }
        }
      },
      (connected) => setWsConnected(connected)
    );

    return cleanup;
  }, []);

  const handleMeasure = async (sensors) => {
    try {
      const res = await api.triggerMeasure(SENSOR_ID, sensors);
      if (res?.error) alert('Chưa đo: ' + res.error);
    } catch (e) {
      console.error('Trigger measure error:', e);
      alert('Không thể gửi lệnh đo tới Server: ' + e.message);
    }
  };

  const handlePump = async (target, state, level = 0, duration = 0) => {
    if (target === 'inlet') setInletOn(state);
    if (target === 'drain') setDrainOn(state);
    try {
      const res = await api.setPump(WATER_ID, target, state, level, duration);
      if (res?.error) throw new Error(res.error);
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
      await api.clearQueue(WATER_ID);
    } catch (e) {
      console.error('Clear queue error:', e);
      alert('Không thể gửi lệnh xóa queue: ' + e.message);
    }
  };

  const handleSetSchedule = async (scheduleData) => {
    try {
      return await api.setSchedule(SENSOR_ID, scheduleData);
    } catch (e) {
      console.error('Set schedule error:', e);
      throw e;
    }
  };

  const handleToggleAuto = async (enabled) => {
    try {
      return await api.toggleAuto(SENSOR_ID, enabled);
    } catch (e) {
      console.error('Toggle auto error:', e);
      throw e;
    }
  };

  const sensorOnline = devices.find((d) => d.id === SENSOR_ID)?.online ?? false;
  const waterOnline = devices.find((d) => d.id === WATER_ID)?.online ?? false;

  return (
    <div className="app-container">
      <Header
        sensorOnline={sensorOnline}
        waterOnline={waterOnline}
        wsConnected={wsConnected}
        currentState={currentState}
      />

      <NavMenu activeTab={activeTab} onChange={setActiveTab} />

      {activeTab === 'dashboard' && (
        <section className="page-section">
          <SensorCards measurement={latestMeasurement} pondConfig={pondConfig} />

          <AdvicePanel
            deviceId={SENSOR_ID}
            pondConfig={pondConfig}
            onPondConfigChange={setPondConfig}
          />

          <SensorTrendChart history={history} />

          <LiveSamplingTimeline
            jobPhase={jobPhase}
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
          <KitLogPanel deviceId={SENSOR_ID} />
        </section>
      )}

      {activeTab === 'schedule' && (
        <section className="page-section">
          <SchedulePanel
            deviceId={SENSOR_ID}
            onSetSchedule={handleSetSchedule}
            onToggleAuto={handleToggleAuto}
          />
        </section>
      )}

      {activeTab === 'calibration' && (
        <section className="page-section">
          <CalibrationPanel
            deviceId={SENSOR_ID}
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
