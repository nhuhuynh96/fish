// Nhiệt độ nước: ưu tiên cảm biến pH RS485 (ph_temp), DS18B20 (temperature) chỉ là dự phòng.
// Backend đã chọn sẵn ở water_temp; bản ghi tới qua WebSocket thì App.jsx gán trước khi dùng.
export function waterTemp(m) {
  if (!m) return null;
  if ('water_temp' in m) return m.water_temp ?? null;
  return m.ph_temp ?? m.temperature ?? null;
}

export function tempSourceLabel(source) {
  if (source === 'ph') return 'cảm biến pH';
  if (source === 'ds18b20') return 'DS18B20 dự phòng';
  return '';
}
