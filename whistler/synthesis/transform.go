package synthesis

var squareRootHalf = float64(float32(0.70710677))

func inverseRealTransform(x []float32, size int, exponent int, sine []float32) {
	half := size / 2
	if exponent > 1 {
		span := 2 * size
		stride := 1
		for range exponent - 1 {
			step := span
			span /= 2
			quarter := span / 4
			eighth := quarter / 2
			stride *= 2
			start := 0
			for {
				for i1 := start; i1 < size; i1 += step {
					i2 := i1 + quarter
					i3 := i2 + quarter
					i4 := i3 + quarter
					x1, x3 := float64(x[i1]), float64(x[i3])
					t1 := x1 - x3
					x[i1] = float32(x3 + x1)
					x[i2] = float32(float64(x[i2]) + float64(x[i2]))
					x4 := float64(x[i4])
					x[i3] = float32(t1 - (x4 + x4))
					x[i4] = float32(t1 - float64(x4*-2.0))
					if quarter == 1 {
						continue
					}
					j1, j2, j3, j4 := i1+eighth, i2+eighth, i3+eighth, i4+eighth
					y1, y2, y3, y4 := float64(x[j1]), float64(x[j2]), float64(x[j3]), float64(x[j4])
					t2 := float64((y4 + y3) * squareRootHalf)
					u1 := float64((y2 - y1) * squareRootHalf)
					x[j1] = float32(y1 + y2)
					x[j2] = float32(y4 - y3)
					x[j3] = float32((t2 + u1) * -2.0)
					x[j4] = float32((u1 - float64(float32(t2))) * 2.0)
				}
				start = 2*step - span
				step *= 4
				if start >= size-1 {
					break
				}
			}
			for j := 1; j < eighth; j++ {
				angle := j * stride
				cc1 := float64(sine[angle+half])
				ss1 := float64(sine[angle])
				cc3 := float64(sine[3*angle+half])
				ss3 := float64(sine[3*angle])
				start := 0
				step := 2 * span
				for {
					for i := start; i < size; i += step {
						i1 := i + j
						i2 := i1 + quarter
						i3 := i2 + quarter
						i4 := i3 + quarter
						i5 := i + quarter - j
						i6 := i5 + quarter
						i7 := i6 + quarter
						i8 := i7 + quarter
						x1, x6 := float64(x[i1]), float64(x[i6])
						t1 := x1 - x6
						x[i1] = float32(x1 + x6)
						x2, x5 := float64(x[i2]), float64(x[i5])
						t2 := x5 - x2
						x[i5] = float32(x2 + x5)
						x3, x8 := float64(x[i3]), float64(x[i8])
						t3 := x3 + x8
						x[i6] = float32(x8 - x3)
						x4, x7 := float64(x[i4]), float64(x[i7])
						t4 := x4 + x7
						x[i2] = float32(x4 - x7)
						u4 := float64(float32(t2 - t3))
						t1f := float64(float32(t1))
						t2f := float64(float32(t2))
						t4f := float64(float32(t4))
						t5 := float64(float32(t1f - t4f))
						u1 := float64(float32(t4f + t1f))
						u2 := float64(float32(t3 + t2f))
						x[i3] = float32(float64(cc1*t5) + float64(ss1*u4))
						x[i7] = float32(float64(ss1*t5) - float64(cc1*u4))
						x[i4] = float32(float64(cc3*u1) - float64(ss3*u2))
						x[i8] = float32(float64(ss3*u1) + float64(cc3*u2))
					}
					start = 2*step - span
					step *= 4
					if start >= size-1 {
						break
					}
				}
			}
		}
	}
	if size-1 > 0 {
		start := 0
		step := 4
		for {
			for i := start; i < size; i += step {
				x0, x1 := float64(x[i]), float64(x[i+1])
				x[i] = float32(x1 + x0)
				x[i+1] = float32(x0 - x1)
			}
			start = 2*step - 2
			step *= 4
			if start >= size-1 {
				break
			}
		}
	}
	j := 0
	for i := 0; i < size-1; i++ {
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
		k := half
		for k <= j {
			j -= k
			k /= 2
		}
		j += k
	}
	divisor := float64(size)
	for i := range x[:size] {
		x[i] = float32(float64(x[i]) / divisor)
	}
}
