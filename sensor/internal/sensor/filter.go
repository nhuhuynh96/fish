package sensor

// MedianFilter lọc trung vị một mảng các giá trị ADC đọc được
// Giúp triệt tiêu hoàn toàn nhiễu gai (spikes) và nhiễu từ nguồn xung
func MedianFilter(samples []int) int {
	n := len(samples)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return samples[0]
	}

	// Sao chép mảng để không làm thay đổi mảng gốc
	buf := make([]int, n)
	copy(buf, samples)

	// Insertion sort đơn giản, tốn rất ít RAM trên vi điều khiển
	for i := 1; i < n; i++ {
		key := buf[i]
		j := i - 1
		for j >= 0 && buf[j] > key {
			buf[j+1] = buf[j]
			j--
		}
		buf[j+1] = key
	}

	return buf[n/2]
}
