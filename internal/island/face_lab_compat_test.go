//go:build render

package island

func faceLidR(p pose) float32 { return min(p.lid+max(p.lidAsym, 0), 1) }
func faceOpen(p pose) float32 { return p.open * p.mouthK }
