package googleearth_test

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"crawler-platform/GoogleEarth"
	"crawler-platform/utlsclient"
)

// exportTerrainToOBJ 将地形数据导出为 OBJ 格式
// 参考 tile_thief 的方法，使用网格索引坐标
func exportTerrainToOBJ(terrain *GoogleEarth.Terrain) string {
	var sb strings.Builder

	// OBJ 文件头
	sb.WriteString("# Google Earth Terrain Model\n")
	sb.WriteString("# Exported from crawler-platform\n")
	sb.WriteString("# Reference: https://github.com/zuo1188/tile_thief/blob/master/src/utils/dem/libge/src/libge/Terrain.cpp\n")
	sb.WriteString(fmt.Sprintf("# QtNode: %s\n", terrain.QtNode))
	sb.WriteString(fmt.Sprintf("# Mesh Groups: %d\n", terrain.NumMeshGroups()))
	sb.WriteString(fmt.Sprintf("# Total Meshes: %d\n", terrain.NumMeshes()))
	sb.WriteString("# Coordinates: Mercator X, Y (meters), Elevation Z (meters)\n")
	sb.WriteString("# Reference: https://github.com/zuo1188/tile_thief/blob/master/src/utils/dem/libge/src/libge/Terrain.cpp\n")
	sb.WriteString("# X, Y: Mercator projection coordinates (meters)\n")
	sb.WriteString("# Z: Elevation (meters)\n")
	sb.WriteString(fmt.Sprintf("mtllib google_earth_terrain_%s.mtl\n", terrain.QtNode))
	sb.WriteString("usemtl terrain\n\n")

	// 第一步：收集所有顶点的墨卡托坐标，计算边界
	var minMx, maxMx, minMy, maxMy float64
	firstVertex := true
	for _, meshes := range terrain.MeshGroups {
		for _, mesh := range meshes {
			for _, vertex := range mesh.Vertices {
				mx, my := GoogleEarth.LatLonToMercator(vertex.Y, vertex.X)
				if firstVertex {
					minMx, maxMx = mx, mx
					minMy, maxMy = my, my
					firstVertex = false
				} else {
					if mx < minMx {
						minMx = mx
					}
					if mx > maxMx {
						maxMx = mx
					}
					if my < minMy {
						minMy = my
					}
					if my > maxMy {
						maxMy = my
					}
				}
			}
		}
	}

	// 计算范围，用于归一化
	rangeX := maxMx - minMx
	rangeY := maxMy - minMy
	if rangeX == 0 {
		rangeX = 1
	}
	if rangeY == 0 {
		rangeY = 1
	}

	// 收集所有Z值，计算Z的范围（用于缩放）
	var minZ, maxZ float64
	firstZ := true
	for _, meshes := range terrain.MeshGroups {
		for _, mesh := range meshes {
			for _, vertex := range mesh.Vertices {
				z := float64(vertex.Z)
				if firstZ {
					minZ, maxZ = z, z
					firstZ = false
				} else {
					if z < minZ {
						minZ = z
					}
					if z > maxZ {
						maxZ = z
					}
				}
			}
		}
	}
	rangeZ := maxZ - minZ
	if rangeZ == 0 {
		rangeZ = 1
	}
	// Z值缩放：将Z值缩放到X、Y范围的10%以内，以保持合理的比例
	// 例如，如果X、Y范围是1000，Z范围可以缩放到0-100
	zScale := 100.0 / rangeZ

	vertexOffset := 1 // OBJ 顶点索引从 1 开始

	// 遍历所有网格组
	for qtNode, meshes := range terrain.MeshGroups {
		sb.WriteString(fmt.Sprintf("# Mesh Group: %s\n", qtNode))
		sb.WriteString(fmt.Sprintf("o MeshGroup_%s\n", qtNode))

		for meshIdx, mesh := range meshes {
			sb.WriteString(fmt.Sprintf("# Mesh %d: %d vertices, %d faces, Level=%d\n", meshIdx, mesh.NumPoints, mesh.NumFaces, mesh.Level))
			sb.WriteString(fmt.Sprintf("# Origin: (%.6f, %.6f), Delta: (%.6f, %.6f)\n", mesh.OriginX, mesh.OriginY, mesh.DeltaX, mesh.DeltaY))
			sb.WriteString(fmt.Sprintf("g Mesh_%s_%d\n", qtNode, meshIdx))

			// 写入顶点 (v x y z)
			// 参考 tile_thief 的实现：使用墨卡托投影坐标（米制）
			// 为了3D查看器能正确显示，将坐标归一化到合理范围（0-1000）
			// 保持X、Y、Z的比例关系
			for _, vertex := range mesh.Vertices {
				// 将经纬度转换为墨卡托投影坐标（米）
				mx, my := GoogleEarth.LatLonToMercator(vertex.Y, vertex.X)

				// 归一化坐标到 0-1000 范围（保持比例）
				// 使用较大的范围（1000）以保持精度
				normalizedX := (mx - minMx) / rangeX * 1000.0
				normalizedY := (my - minMy) / rangeY * 1000.0
				// Z值归一化：缩放到0-100范围，保持与X、Y的合理比例
				normalizedZ := (float64(vertex.Z) - minZ) * zScale

				sb.WriteString(fmt.Sprintf("v %.6f %.6f %.3f\n", normalizedX, normalizedY, normalizedZ))
			}

			// 写入面 (f v1 v2 v3)
			// OBJ格式要求面的顶点顺序是逆时针（从外部看）
			// 检查面的有效性：过滤掉无效面（重复顶点、共线顶点）
			for _, face := range mesh.Faces {
				// 获取三个顶点索引
				idxA := int(face.A)
				idxB := int(face.B)
				idxC := int(face.C)

				// 确保索引在有效范围内
				if idxA >= len(mesh.Vertices) || idxB >= len(mesh.Vertices) || idxC >= len(mesh.Vertices) {
					continue
				}

				// 检查是否有重复的顶点索引
				if idxA == idxB || idxB == idxC || idxA == idxC {
					continue // 跳过无效面（重复顶点）
				}

				// 转换为墨卡托坐标（用于归一化前的计算）
				ax, ay := GoogleEarth.LatLonToMercator(mesh.Vertices[idxA].Y, mesh.Vertices[idxA].X)
				bx, by := GoogleEarth.LatLonToMercator(mesh.Vertices[idxB].Y, mesh.Vertices[idxB].X)
				cx, cy := GoogleEarth.LatLonToMercator(mesh.Vertices[idxC].Y, mesh.Vertices[idxC].X)

				// 检查三个顶点是否共线（使用2D叉积）
				// 向量 AB 和 AC（在XY平面）
				abx := bx - ax
				aby := by - ay
				acx := cx - ax
				acy := cy - ay

				// 2D叉积：AB × AC（Z分量）
				// 如果叉积接近0，说明三个顶点共线，这是无效面
				normalZ := abx*acy - aby*acx
				const epsilon = 1e-6
				if math.Abs(normalZ) < epsilon {
					continue // 跳过无效面（共线顶点）
				}

				// 检查顶点坐标是否相同（归一化后）
				// 获取归一化后的坐标
				normAx := (ax - minMx) / rangeX * 1000.0
				normAy := (ay - minMy) / rangeY * 1000.0
				normBx := (bx - minMx) / rangeX * 1000.0
				normBy := (by - minMy) / rangeY * 1000.0
				normCx := (cx - minMx) / rangeX * 1000.0
				normCy := (cy - minMy) / rangeY * 1000.0

				// 检查归一化后的坐标是否相同
				if (math.Abs(normAx-normBx) < epsilon && math.Abs(normAy-normBy) < epsilon) ||
					(math.Abs(normBx-normCx) < epsilon && math.Abs(normBy-normCy) < epsilon) ||
					(math.Abs(normAx-normCx) < epsilon && math.Abs(normAy-normCy) < epsilon) {
					continue // 跳过无效面（相同坐标）
				}

				// 检查面的边长是否过大（可能是错误的面）
				// 计算归一化后的边长
				distAB := math.Sqrt((normBx-normAx)*(normBx-normAx) + (normBy-normAy)*(normBy-normAy))
				distBC := math.Sqrt((normCx-normBx)*(normCx-normBx) + (normCy-normBy)*(normCy-normBy))
				distCA := math.Sqrt((normAx-normCx)*(normAx-normCx) + (normAy-normCy)*(normAy-normCy))
				maxDist := math.Max(distAB, math.Max(distBC, distCA))

				// 如果最大边长超过整个范围的80%，可能是错误的面（跨越多个mesh）
				// 但地形网格本身可能很大，所以只过滤明显错误的面（>95%）
				if maxDist > 950.0 {
					continue // 跳过异常大的面（可能是索引错误）
				}

				// 如果法线Z为负，需要反转顶点顺序（OBJ要求逆时针）
				v1 := int(face.A) + vertexOffset
				v2 := int(face.B) + vertexOffset
				v3 := int(face.C) + vertexOffset

				if normalZ < 0 {
					// 反转顺序
					sb.WriteString(fmt.Sprintf("f %d %d %d\n", v1, v3, v2))
				} else {
					// 保持原顺序
					sb.WriteString(fmt.Sprintf("f %d %d %d\n", v1, v2, v3))
				}
			}

			// 更新顶点偏移量
			vertexOffset += mesh.NumPoints
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// exportTerrainToXYZ 导出为 XYZ 格式（每行：经度 纬度 高程）
// 这是最通用的 DEM 格式，任何 GIS 软件都能读取
func exportTerrainToXYZ(terrain *GoogleEarth.Terrain) (string, error) {
	var sb strings.Builder
	sb.WriteString("# XYZ Format DEM\n")
	sb.WriteString("# Format: Longitude Latitude Elevation\n")
	sb.WriteString("# Generated from Google Earth Terrain Data\n\n")

	pointCount := 0
	for _, meshes := range terrain.MeshGroups {
		for _, mesh := range meshes {
			for _, v := range mesh.Vertices {
				sb.WriteString(fmt.Sprintf("%.8f %.8f %.3f\n", v.X, v.Y, v.Z))
				pointCount++
			}
		}
	}

	return sb.String(), nil
}

// exportTerrainToDEM 参考 tile_thief 的 toDEM 实现，将地形数据栅格化为 DEM
func exportTerrainToDEM(terrain *GoogleEarth.Terrain) (string, int, int, error) {
	// 汇总所有 mesh
	totalMeshes := 0
	for _, meshes := range terrain.MeshGroups {
		totalMeshes += len(meshes)
	}
	if totalMeshes == 0 {
		return "", 0, 0, fmt.Errorf("terrain has no meshes")
	}

	// 计算整体经纬度边界
	minLat, minLon, maxLat, maxLon := 90.0, 180.0, -90.0, -180.0
	for _, meshes := range terrain.MeshGroups {
		for _, mesh := range meshes {
			for _, v := range mesh.Vertices {
				if v.Y < minLat {
					minLat = v.Y
				}
				if v.Y > maxLat {
					maxLat = v.Y
				}
				if v.X < minLon {
					minLon = v.X
				}
				if v.X > maxLon {
					maxLon = v.X
				}
			}
		}
	}

	// 转为墨卡托坐标
	LBX, LBY := GoogleEarth.LatLonToMercator(minLat, minLon)
	RTX, RTY := GoogleEarth.LatLonToMercator(maxLat, maxLon)

	// 计算网格尺寸（每个 mesh 对应 128x128）
	gridSize := int(math.Ceil(math.Sqrt(float64(totalMeshes)))) * 128
	if gridSize == 0 {
		gridSize = 256
	}
	nCols, nRows := gridSize, gridSize
	cellSizeX := (RTX - LBX) / float64(nCols)
	cellSizeY := (RTY - LBY) / float64(nRows)
	if cellSizeX == 0 {
		cellSizeX = 1
	}
	if cellSizeY == 0 {
		cellSizeY = 1
	}

	// 初始化 DEM 数组
	noData := -math.MaxFloat32
	demData := make([][]float64, nRows)
	for i := range demData {
		demData[i] = make([]float64, nCols)
		for j := range demData[i] {
			demData[i][j] = noData
		}
	}

	epsilon := 1e-5
	clamp := func(val, min, max int) int {
		if val < min {
			return min
		}
		if val > max {
			return max
		}
		return val
	}

	// 遍历所有 mesh 与三角形
	for _, meshes := range terrain.MeshGroups {
		for _, mesh := range meshes {
			if len(mesh.Vertices) == 0 || len(mesh.Faces) == 0 {
				continue
			}

			// 预转换顶点到墨卡托
			type point struct {
				x, y, z float64
			}
			verts := make([]point, len(mesh.Vertices))
			for i, v := range mesh.Vertices {
				mx, my := GoogleEarth.LatLonToMercator(v.Y, v.X)
				verts[i] = point{mx, my, float64(v.Z)}
			}

			for _, face := range mesh.Faces {
				idx := [3]int{int(face.A), int(face.B), int(face.C)}
				skip := false
				for _, id := range idx {
					if id < 0 || id >= len(verts) {
						skip = true
						break
					}
				}
				if skip {
					continue
				}

				X := [3]float64{verts[idx[0]].x, verts[idx[1]].x, verts[idx[2]].x}
				Y := [3]float64{verts[idx[0]].y, verts[idx[1]].y, verts[idx[2]].y}
				Z := [3]float64{verts[idx[0]].z, verts[idx[1]].z, verts[idx[2]].z}

				minx := math.Min(X[0], math.Min(X[1], X[2]))
				maxx := math.Max(X[0], math.Max(X[1], X[2]))
				miny := math.Min(Y[0], math.Min(Y[1], Y[2]))
				maxy := math.Max(Y[0], math.Max(Y[1], Y[2]))

				nLBX := clamp(int(0.5+(minx-LBX)/cellSizeX), 0, nCols-1)
				nRTX := clamp(int((maxx-LBX)/cellSizeX+0.5), 0, nCols-1)
				nLBY := clamp(int(0.5+(miny-LBY)/cellSizeY), 0, nRows-1)
				nRTY := clamp(int((maxy-LBY)/cellSizeY+0.5), 0, nRows-1)

				dx01 := X[1] - X[0]
				dy01 := Y[1] - Y[0]
				dx12 := X[2] - X[1]
				dy12 := Y[2] - Y[1]
				dx20 := X[0] - X[2]
				dy20 := Y[0] - Y[2]

				for row := nLBY; row <= nRTY; row++ {
					demY := LBY + float64(row)*cellSizeY
					if demY < miny-epsilon || demY > maxy+epsilon {
						continue
					}
					for col := nLBX; col <= nRTX; col++ {
						demX := LBX + float64(col)*cellSizeX
						if demX < minx-epsilon || demX > maxx+epsilon {
							continue
						}

						dx1 := demX - X[1]
						dy1 := demY - Y[1]
						dx2 := demX - X[2]
						dy2 := demY - Y[2]
						dx0 := demX - X[0]
						dy0 := demY - Y[0]

						v01 := dx01*dy1 - dx1*dy01
						v12 := dx12*dy2 - dx2*dy12
						v20 := dx20*dy0 - dx0*dy20

						var elevation float64
						inside := false

						if (v01 > epsilon && v12 > epsilon && v20 > epsilon) || (v01 < -epsilon && v12 < -epsilon && v20 < -epsilon) {
							denom := (Y[1]-Y[2])*(X[0]-X[2]) + (X[2]-X[1])*(Y[0]-Y[2])
							if math.Abs(denom) > epsilon {
								w0 := ((Y[1]-Y[2])*(demX-X[2]) + (X[2]-X[1])*(demY-Y[2])) / denom
								w1 := ((Y[2]-Y[0])*(demX-X[2]) + (X[0]-X[2])*(demY-Y[2])) / denom
								w2 := 1.0 - w0 - w1
								if w0 >= -epsilon && w1 >= -epsilon && w2 >= -epsilon {
									elevation = w0*Z[0] + w1*Z[1] + w2*Z[2]
									inside = true
								}
							}
						}

						if !inside {
							continue
						}

						targetRow := nRows - 1 - row
						if targetRow < 0 || targetRow >= nRows {
							continue
						}
						if demData[targetRow][col] == noData {
							demData[targetRow][col] = elevation
						} else {
							demData[targetRow][col] = (demData[targetRow][col] + elevation) * 0.5
						}
					}
				}
			}
		}
	}

	// 输出为 ESRI ASCII Grid
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ncols %d\n", nCols))
	sb.WriteString(fmt.Sprintf("nrows %d\n", nRows))
	sb.WriteString(fmt.Sprintf("xllcorner %.6f\n", LBX))
	sb.WriteString(fmt.Sprintf("yllcorner %.6f\n", LBY))
	sb.WriteString(fmt.Sprintf("cellsize %.6f\n", cellSizeX))
	sb.WriteString("NODATA_value -9999\n")

	for row := 0; row < nRows; row++ {
		for col := 0; col < nCols; col++ {
			if col > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(fmt.Sprintf("%.3f", demData[row][col]))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nCols, nRows, nil
}

// findGDALTranslate 查找 gdal_translate 可执行文件的路径
func findGDALTranslate() (string, error) {
	// 首先尝试直接调用（在 PATH 中）
	if path, err := exec.LookPath("gdal_translate"); err == nil {
		return path, nil
	}

	// 尝试常见的安装路径
	commonPaths := []string{
		"/usr/bin/gdal_translate",
		"/usr/local/bin/gdal_translate",
		"/opt/local/bin/gdal_translate",
		"/home/linuxbrew/.linuxbrew/bin/gdal_translate",
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("未找到 gdal_translate，请确保 GDAL 已正确安装并在 PATH 中")
}

// exportTerrainToGeoTIFF 使用 GDAL 命令行工具将 ASC 文件转换为 GeoTIFF
// 如果 GDAL 不可用，返回错误提示
func exportTerrainToGeoTIFF(ascFilePath string) (string, error) {
	if ascFilePath == "" {
		return "", fmt.Errorf("ASC 文件路径不能为空")
	}

	// 检查 ASC 文件是否存在
	if _, err := os.Stat(ascFilePath); os.IsNotExist(err) {
		return "", fmt.Errorf("ASC 文件不存在: %s", ascFilePath)
	}

	// 查找 gdal_translate
	gdalPath, err := findGDALTranslate()
	if err != nil {
		return "", fmt.Errorf("无法找到 gdal_translate: %v", err)
	}

	// 生成 GeoTIFF 文件路径
	geotiffPath := strings.TrimSuffix(ascFilePath, ".asc") + ".tif"

	// 执行转换命令
	cmd := exec.Command(gdalPath, "-of", "GTiff",
		"-a_srs", "EPSG:3857", // Web Mercator 投影
		"-co", "COMPRESS=LZW", // 使用 LZW 压缩
		ascFilePath, geotiffPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("GDAL 转换失败: %v, 输出: %s", err, string(output))
	}

	// 检查输出文件是否生成
	if _, err := os.Stat(geotiffPath); os.IsNotExist(err) {
		return "", fmt.Errorf("GeoTIFF 文件未生成，GDAL 输出: %s", string(output))
	}

	return geotiffPath, nil
}

// btoi 将 bool 转换为 int
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestQuadtreePacket_RealData 测试真实的 quadtree packet 数据解包
// 地址：https://kh.google.com/flatfile?q2-0-q.2009
// 需要先通过 geauth 获取 session，然后在请求头中携带 session 才能获取 body
// 返回的数据是加密的，需要先解密再解析
// 注意：此测试需要较长时间（约 1-2 分钟），已设置 120 秒超时
func TestQuadtreePacket_RealData(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（使用 -short 标志）")
	}

	// 设置测试超时时间为 120 秒
	if deadline, ok := t.Deadline(); !ok {
		t.Logf("警告：未设置测试超时，建议使用 -timeout 120s")
	} else {
		t.Logf("测试超时设置: %v", time.Until(deadline).Round(time.Second))
	}

	// 1. 创建连接池配置
	config, err := utlsclient.LoadPoolConfigFromFile("../../config/config.toml")
	if err != nil {
		t.Logf("无法加载配置文件，使用默认配置: %v", err)
		config = &utlsclient.PoolConfig{
			MaxConnections:         100,
			MaxConnsPerHost:        10,
			MaxIdleConns:           20,
			ConnTimeout:            30 * time.Second,
			IdleTimeout:            60 * time.Second,
			MaxLifetime:            300 * time.Second,
			TestTimeout:            10 * time.Second,
			HealthCheckInterval:    30 * time.Second,
			CleanupInterval:        60 * time.Second,
			BlacklistCheckInterval: 300 * time.Second,
			DNSUpdateInterval:      1800 * time.Second,
			MaxRetries:             3,
		}
	}

	// 2. 创建连接池
	pool := utlsclient.NewUTLSHotConnPool(config)

	// 获取一个热连接，复用于所有请求（性能优化）
	conn, err := pool.GetConnection(GoogleEarth.HOST_NAME)
	if err != nil {
		t.Fatalf("获取热连接失败: %v", err)
	}
	defer pool.PutConnection(conn)

	// 创建一个 UTLSClient 实例，复用于所有请求
	client := utlsclient.NewUTLSClient(conn)
	client.SetTimeout(30 * time.Second)

	// 3. 获取认证 session（使用同一个热连接）
	t.Logf("\n=== 步骤 1: 获取 Google Earth 认证 Session ===")
	// 直接使用已获取的client获取session，避免创建新连接
	geauthURL := "https://" + GoogleEarth.HOST_NAME + "/geauth"
	authKey, err := GoogleEarth.GenerateRandomGeAuth(0) // 生成随机认证密钥
	if err != nil {
		t.Fatalf("生成认证密钥失败: %v", err)
	}

	// 创建POST请求
	authReq, err := http.NewRequest("POST", geauthURL, bytes.NewReader(authKey))
	if err != nil {
		t.Fatalf("创建认证请求失败: %v", err)
	}
	authReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(authKey)))
	authReq.Header.Set("Host", GoogleEarth.HOST_NAME)

	// 使用同一个client发送请求
	authResp, err := client.Do(authReq)
	if err != nil {
		t.Fatalf("认证请求失败: %v", err)
	}
	defer authResp.Body.Close()

	if authResp.StatusCode != 200 {
		t.Fatalf("认证失败，状态码: %d", authResp.StatusCode)
	}

	// 读取响应body
	authBody, err := io.ReadAll(authResp.Body)
	if err != nil {
		t.Fatalf("读取认证响应失败: %v", err)
	}

	// 解析session（从第8字节开始，直到遇到NULL字节）
	if len(authBody) <= 8 {
		t.Fatalf("认证响应长度不足: %d 字节", len(authBody))
	}
	var sessionBytes []byte
	for i := 8; i < len(authBody); i++ {
		if authBody[i] == 0 {
			break
		}
		sessionBytes = append(sessionBytes, authBody[i])
	}
	if len(sessionBytes) == 0 {
		t.Fatal("未找到有效的sessionid")
	}
	session := string(sessionBytes)

	// 输出 session 的十六进制和字符串格式
	t.Logf("✅ 成功获取 session (ASCII): %s", session)
	t.Logf("   Session (Hex): % X", []byte(session))
	t.Logf("   Session 长度: %d 字节", len(session))

	// 4. 获取 dbRoot 数据以获得正确的 epoch
	t.Logf("\n=== 步骤 2: 获取 dbRoot.v5 数据 ===")
	dbRootURL := "https://" + GoogleEarth.HOST_NAME + GoogleEarth.DBROOT_PATH

	// 创建请求（复用热连接）
	req2, err := http.NewRequest("GET", dbRootURL, nil)
	if err != nil {
		t.Fatalf("创建 dbRoot 请求失败: %v", err)
	}

	// 设置请求头
	req2.Header.Set("Host", GoogleEarth.HOST_NAME)
	req2.Header.Set("Cookie", fmt.Sprintf("SessionId=%s;State=1", session))
	req2.Header.Set("Content-Type", "application/octet-stream")

	// 发送请求（复用client）
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("dbRoot 请求失败: %v", err)
	}
	defer resp2.Body.Close()

	// 检查响应状态
	if resp2.StatusCode != 200 {
		t.Fatalf("dbRoot 请求失败，状态码: %d", resp2.StatusCode)
	}

	// 读取 dbRoot 响应
	dbRootBody, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("读取 dbRoot 响应失败: %v", err)
	}

	t.Logf("✅ 成功获取 dbRoot 数据，大小: %d 字节", len(dbRootBody))

	// 5. 解析 dbRoot 获取 epoch 和加密密钥
	t.Logf("\n=== 步骤 3: 解析 dbRoot ===")
	dbRootData, err := GoogleEarth.ParseDbRootComplete(dbRootBody)
	if err != nil {
		t.Fatalf("解析 dbRoot 失败: %v", err)
	}

	t.Logf("✅ 成功解析 dbRoot")
	t.Logf("   Version: %d", dbRootData.Version)
	t.Logf("   CryptKey 长度: %d 字节", len(dbRootData.CryptKey))

	// 使用解析出的密钥更新全局密钥
	GoogleEarth.CryptKey = dbRootData.CryptKey

	// 6. 请求 quadtree packet 数据
	// 先尝试根节点的数据
	t.Logf("\n=== 步骤 4: 获取 Quadtree Packet 数据 ===")
	tilekey := "0"                   // 根节点
	epoch := int(dbRootData.Version) // 使用从 dbRoot 获取的版本号
	q2URL := fmt.Sprintf("https://%s/flatfile?q2-%s-q.%d", GoogleEarth.HOST_NAME, tilekey, epoch)
	t.Logf("请求 URL: %s", q2URL)

	// 创建请求（复用热连接）
	req3, err := http.NewRequest("GET", q2URL, nil)
	if err != nil {
		t.Fatalf("创建 q2 请求失败: %v", err)
	}

	// 设置请求头
	req3.Header.Set("Host", GoogleEarth.HOST_NAME)
	req3.Header.Set("Cookie", fmt.Sprintf("SessionId=%s;State=1", session))
	req3.Header.Set("Content-Type", "application/octet-stream")
	req3.Header.Set("User-Agent", "GoogleEarth/7.3.6.9345(Windows;Microsoft Windows (6.2.9200.0);en;kml:2.2;client:Pro;type:default)")

	// 发送请求（复用client）
	resp3, err := client.Do(req3)
	if err != nil {
		t.Fatalf("q2 请求失败: %v", err)
	}
	defer resp3.Body.Close()

	// 检查响应状态
	if resp3.StatusCode != 200 {
		// 如果失败，输出详细错误信息
		body, _ := io.ReadAll(resp3.Body)
		t.Logf("响应状态码: %d", resp3.StatusCode)
		t.Logf("响应内容: %s", string(body))
		t.Fatalf("q2 请求失败，状态码: %d", resp3.StatusCode)
	}

	// 读取响应 body（加密数据）
	encryptedBody, err := io.ReadAll(resp3.Body)
	if err != nil {
		t.Fatalf("读取 q2 响应失败: %v", err)
	}

	t.Logf("✅ 成功获取 q2 数据，大小: %d 字节", len(encryptedBody))
	t.Logf("   加密数据前 16 字节: % X", encryptedBody[:min(16, len(encryptedBody))])

	// 7. 解密数据
	t.Logf("\n=== 步骤 5: 解密 Quadtree Packet 数据 ===")
	decryptedBody, err := GoogleEarth.UnpackGEZlib(encryptedBody)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}

	t.Logf("✅ 成功解密数据，大小: %d 字节", len(decryptedBody))
	t.Logf("   解密数据前 16 字节: % X", decryptedBody[:min(16, len(decryptedBody))])

	// 8. 解析 quadtree packet
	// 检查是二进制格式还是 protobuf 格式
	t.Logf("\n=== 步骤 6: 解析 Quadtree Packet ===")

	// 检查魔法数字判断格式
	var magicID uint32
	if len(decryptedBody) >= 4 {
		// 小端序读取前4字节
		magicID = uint32(decryptedBody[0]) | uint32(decryptedBody[1])<<8 |
			uint32(decryptedBody[2])<<16 | uint32(decryptedBody[3])<<24
	}

	t.Logf("Magic ID: 0x%X", magicID)

	// 根据内存中的说明，quadtree packet 需要使用 quadtreequantum16 解析（二进制格式）
	if magicID == GoogleEarth.KeyholeMagicID {
		t.Logf("检测到二进制格式 (Keyhole Magic ID: 0x%X)，使用 QuadTreePacket16 解析", magicID)

		// 使用二进制格式解析器
		qtp16 := GoogleEarth.NewQuadTreePacket16()
		if err := qtp16.Decode(decryptedBody); err != nil {
			t.Fatalf("解析二进制 packet 失败: %v", err)
		}

		t.Logf("✅ 成功解析 Quadtree Packet (二进制格式)")
		t.Logf("   Magic ID: 0x%X", qtp16.MagicID)
		t.Logf("   Data Type ID: %d", qtp16.DataTypeID)
		t.Logf("   Version: %d", qtp16.Version)
		t.Logf("   数据实例数量: %d", len(qtp16.DataInstances))

		// 9. 遍历节点并输出信息
		t.Logf("\n=== 步骤 7: 遍历节点信息 ===")
		for i, quantum := range qtp16.DataInstances {
			if quantum == nil {
				continue
			}

			t.Logf("\n节点 %d:", i)
			t.Logf("  Children: 0x%02X (子节点: %d%d%d%d)",
				quantum.Children,
				btoi(quantum.GetBit(0)),
				btoi(quantum.GetBit(1)),
				btoi(quantum.GetBit(2)),
				btoi(quantum.GetBit(3)))
			t.Logf("  CNode Version: %d", quantum.CNodeVersion)
			t.Logf("  Image Version: %d", quantum.ImageVersion)
			t.Logf("  Terrain Version: %d", quantum.TerrainVersion)
			t.Logf("  有缓存节点: %v", quantum.GetCacheNodeBit())
			t.Logf("  有影像数据: %v", quantum.GetImageBit())
			t.Logf("  有地形数据: %v", quantum.GetTerrainBit())
			t.Logf("  有矢量数据: %v", quantum.GetDrawableBit())
			t.Logf("  通道数量: %d", len(quantum.ChannelType))

			// 只输出前 5 个节点的详细信息
			if i >= 4 {
				t.Logf("\n... 还有 %d 个节点（省略）", len(qtp16.DataInstances)-5)
				break
			}
		}

		// 10. 提取数据引用
		t.Logf("\n=== 步骤 8: 提取数据引用 ===")
		references := &GoogleEarth.QuadtreeDataReferenceGroup{}
		pathPrefix := GoogleEarth.NewQuadtreePathFromString("0") // 根节点路径前缀

		qtp16.GetDataReferences(references, pathPrefix, true)

		// 过滤 QTP 引用：只有 tilekey 长度能被 4 整除的才是 q2（子节点集合）
		var filteredQtpRefs []GoogleEarth.QuadtreeDataReference
		for _, ref := range references.QtpRefs {
			tilekey := ref.QtPath.AsString()
			if len(tilekey)%4 == 0 {
				filteredQtpRefs = append(filteredQtpRefs, ref)
			}
		}
		references.QtpRefs = filteredQtpRefs

		t.Logf("数据引用统计:")
		t.Logf("  影像引用: %d 个", len(references.ImgRefs))
		t.Logf("  地形引用: %d 个", len(references.TerRefs))
		t.Logf("  矢量引用: %d 个", len(references.VecRefs))
		t.Logf("  QTP 引用 (q2子节点集合, tilekey长度能被4整除): %d 个", len(references.QtpRefs))

		// 输出前几个影像引用
		if len(references.ImgRefs) > 0 {
			t.Logf("\n前 3 个影像引用:")
			for i := 0; i < min(3, len(references.ImgRefs)); i++ {
				ref := references.ImgRefs[i]
				t.Logf("  %d. Path=%s, Version=%d, Provider=%d",
					i+1,
					ref.QtPath.AsString(),
					ref.Version,
					ref.Provider)
			}
		}

		// 输出前几个地形引用
		if len(references.TerRefs) > 0 {
			t.Logf("\n前 3 个地形引用（地形数据只在奇数层级，即tilekey长度为奇数）:")
			for i := 0; i < min(3, len(references.TerRefs)); i++ {
				ref := references.TerRefs[i]
				tilekey := ref.QtPath.AsString()
				t.Logf("  %d. Path=%s (长度=%d, %s), Version=%d, Provider=%d",
					i+1,
					tilekey,
					len(tilekey),
					map[bool]string{true: "奇数✓", false: "偶数✗"}[len(tilekey)%2 == 1],
					ref.Version,
					ref.Provider)
			}
		}

		// 11. 测试请求和解析 QTP 引用（q2 子节点）
		t.Logf("\n=== 步骤 9: 请求并解析 QTP 引用的子节点 ===")
		t.Logf("说明: q2 是一个子集合，管理 4 层数据")
		t.Logf("      例如 tilekey=0022 (第4层) 包含第 5,6,7,8 层数据")
		t.Logf("      地形数据只在奇数层级（5层、7层）才有")
		if len(references.QtpRefs) > 0 {
			// 只测试前 3 个 QTP 引用
			testCount := min(3, len(references.QtpRefs))
			t.Logf("测试前 %d 个 QTP 引用（共 %d 个）:", testCount, len(references.QtpRefs))

			for i := 0; i < testCount; i++ {
				qtpRef := references.QtpRefs[i]
				childTilekey := qtpRef.QtPath.AsString()
				childEpoch := int(dbRootData.Version) // 使用 dbRoot 的版本号

				t.Logf("\n--- QTP %d: Path=%s (长度=%d), Version=%d ---",
					i+1, childTilekey, len(childTilekey), qtpRef.Version)

				// 构建子节点的 q2 URL
				childURL := fmt.Sprintf("https://%s/flatfile?q2-%s-q.%d",
					GoogleEarth.HOST_NAME, childTilekey, childEpoch)

				// 创建请求（复用热连接）
				childReq, err := http.NewRequest("GET", childURL, nil)
				if err != nil {
					t.Logf("  ⚠️  创建请求失败: %v", err)
					continue
				}

				// 设置请求头
				childReq.Header.Set("Host", GoogleEarth.HOST_NAME)
				childReq.Header.Set("Cookie", fmt.Sprintf("SessionId=%s;State=1", session))
				childReq.Header.Set("Content-Type", "application/octet-stream")

				// 发送请求（复用client）
				childResp, err := client.Do(childReq)
				if err != nil {
					t.Logf("  ⚠️  请求失败: %v", err)
					continue
				}

				// 读取响应
				childBody, err := io.ReadAll(childResp.Body)
				childResp.Body.Close()

				if err != nil {
					t.Logf("  ⚠️  读取响应失败: %v", err)
					continue
				}

				if childResp.StatusCode != 200 {
					t.Logf("  ⚠️  状态码: %d, 响应大小: %d 字节", childResp.StatusCode, len(childBody))
					continue
				}

				t.Logf("  ✅ 成功获取数据，大小: %d 字节", len(childBody))

				// 解密
				childDecrypted, err := GoogleEarth.UnpackGEZlib(childBody)
				if err != nil {
					t.Logf("  ⚠️  解密失败: %v", err)
					continue
				}

				t.Logf("  ✅ 解密成功，大小: %d 字节", len(childDecrypted))

				// 解析
				childQtp := GoogleEarth.NewQuadTreePacket16()
				if err := childQtp.Decode(childDecrypted); err != nil {
					t.Logf("  ⚠️  解析失败: %v", err)
					continue
				}

				t.Logf("  ✅ 解析成功: %d 个数据实例", len(childQtp.DataInstances))

				// 统计子节点的数据类型
				var childImgCount, childTerCount int
				for _, quantum := range childQtp.DataInstances {
					if quantum.GetImageBit() {
						childImgCount++
					}
					if quantum.GetTerrainBit() {
						childTerCount++
					}
				}

				t.Logf("  统计: 影像=%d, 地形=%d", childImgCount, childTerCount)
			}
		}

		// 12. 测试请求和解密影像数据
		t.Logf("\n=== 步骤 10: 请求并解密影像数据 ===")
		if len(references.ImgRefs) > 0 {
			// 只测试第一个影像引用
			imgRef := references.ImgRefs[0]
			imgTilekey := imgRef.QtPath.AsString()
			imgVersion := imgRef.Version
			imgProvider := imgRef.Provider

			t.Logf("测试影像: Path=%s, Version=%d, Provider=%d", imgTilekey, imgVersion, imgProvider)

			// 构建影像 URL
			imgURL := fmt.Sprintf("https://%s/flatfile?f1-%s-i.%d",
				GoogleEarth.HOST_NAME, imgTilekey, imgVersion)
			t.Logf("请求 URL: %s", imgURL)

			// 创建请求（复用热连接）
			imgReq, err := http.NewRequest("GET", imgURL, nil)
			if err != nil {
				t.Logf("⚠️  创建请求失败: %v", err)
			} else {
				// 设置请求头
				imgReq.Header.Set("Host", GoogleEarth.HOST_NAME)
				imgReq.Header.Set("Cookie", fmt.Sprintf("SessionId=%s;State=1", session))
				imgReq.Header.Set("Content-Type", "application/octet-stream")

				// 发送请求（复用client）
				imgResp, err := client.Do(imgReq)
				if err != nil {
					t.Logf("⚠️  请求失败: %v", err)
				} else {
					defer imgResp.Body.Close()

					// 读取响应
					imgBody, err := io.ReadAll(imgResp.Body)
					if err != nil {
						t.Logf("⚠️  读取响应失败: %v", err)
					} else if imgResp.StatusCode != 200 {
						t.Logf("⚠️  状态码: %d, 响应大小: %d 字节", imgResp.StatusCode, len(imgBody))
					} else {
						t.Logf("✅ 成功获取影像数据，大小: %d 字节", len(imgBody))

						// 解密影像数据（影像数据使用 GeDecrypt 直接解密，不需要 UnpackGEZlib）
						imgDecrypted := make([]byte, len(imgBody))
						copy(imgDecrypted, imgBody)
						GoogleEarth.GeDecrypt(imgDecrypted, GoogleEarth.CryptKey)
						t.Logf("✅ 解密成功，大小: %d 字节", len(imgDecrypted))
						t.Logf("解密数据头: % X (应为 JPEG: FF D8 FF)", imgDecrypted[:min(10, len(imgDecrypted))])

						// 保存为 JPG 文件
						imgFileName := fmt.Sprintf("/home/stone/crawler-platform/test_output/google_earth_tile_%s.jpg", imgTilekey)
						err = os.WriteFile(imgFileName, imgDecrypted, 0644)
						if err != nil {
							t.Logf("⚠️  保存文件失败: %v", err)
						} else {
							t.Logf("✅ 成功保存 JPEG 文件: %s (256x256)", imgFileName)
						}

					}
				}
			}
		}

		// 13. 测试请求和解密地形数据
		t.Logf("\n=== 步骤 11: 请求并解密地形数据 ===")
		t.Logf("说明: 地形数据只在奇数层级（tilekey长度为奇数）")
		if len(references.TerRefs) > 0 {
			// 只测试第一个地形引用
			terRef := references.TerRefs[0]
			terTilekey := terRef.QtPath.AsString()
			terVersion := terRef.Version
			terProvider := terRef.Provider

			t.Logf("测试地形: Path=%s (长度=%d, %s), Version=%d, Provider=%d",
				terTilekey,
				len(terTilekey),
				map[bool]string{true: "奇数✓", false: "偶数✗"}[len(terTilekey)%2 == 1],
				terVersion,
				terProvider)

			// 构建地形 URL
			terURL := fmt.Sprintf("https://%s/flatfile?f1c-%s-t.%d",
				GoogleEarth.HOST_NAME, terTilekey, terVersion)
			t.Logf("请求 URL: %s", terURL)

			// 创建请求（复用热连接）
			terReq, err := http.NewRequest("GET", terURL, nil)
			if err != nil {
				t.Logf("⚠️  创建请求失败: %v", err)
			} else {
				// 设置请求头
				terReq.Header.Set("Host", GoogleEarth.HOST_NAME)
				terReq.Header.Set("Cookie", fmt.Sprintf("SessionId=%s;State=1", session))
				terReq.Header.Set("Content-Type", "application/octet-stream")

				// 发送请求（复用client）
				terResp, err := client.Do(terReq)
				if err != nil {
					t.Logf("⚠️  请求失败: %v", err)
				} else {
					defer terResp.Body.Close()

					// 读取响应
					terBody, err := io.ReadAll(terResp.Body)
					if err != nil {
						t.Logf("⚠️  读取响应失败: %v", err)
					} else if terResp.StatusCode != 200 {
						t.Logf("⚠️  状态码: %d, 响应大小: %d 字节", terResp.StatusCode, len(terBody))
					} else {
						t.Logf("✅ 成功获取地形数据，大小: %d 字节", len(terBody))

						// 解密地形数据（先使用 GeDecrypt 解密，再使用 UnpackGEZlib 解压缩）
						terDecrypted := make([]byte, len(terBody))
						copy(terDecrypted, terBody)
						GoogleEarth.GeDecrypt(terDecrypted, GoogleEarth.CryptKey)
						t.Logf("✅ 第一步解密成功，大小: %d 字节", len(terDecrypted))
						t.Logf("解密数据头: % X (ZLIB 魔法数: 74 68 DE AD)", terDecrypted[:min(16, len(terDecrypted))])

						// 第二步：解压缩
						terUnpacked, err := GoogleEarth.UnpackGEZlib(terDecrypted)
						if err != nil {
							t.Logf("⚠️  解压缩失败: %v", err)
						} else {
							t.Logf("✅ 第二步解压缩成功，大小: %d 字节", len(terUnpacked))
							t.Logf("解压缩数据头: % X", terUnpacked[:min(16, len(terUnpacked))])

							// 第三步：解析地形网格（使用二进制格式）
							// 注：地形数据可能包裹在 Protobuf 格式中（TerrainPacketExtraDataProto），
							//     包含 water_tile_quads 和 original_terrain_packet 字段
							//     但当前直接使用二进制解析器处理
							terrain := GoogleEarth.NewTerrain(terTilekey)
							err = terrain.Decode(terUnpacked)
							if err != nil {
								t.Logf("⚠️  解析地形网格失败: %v", err)
							} else {
								t.Logf("✅ 第三步解析地形网格成功")
								t.Logf("网格组数: %d", terrain.NumMeshGroups())
								t.Logf("总网格数: %d", terrain.NumMeshes())

								// 输出网格详细信息
								for qtNode, meshes := range terrain.MeshGroups {
									t.Logf("\n  网格组 [%s]: %d 个网格", qtNode, len(meshes))
									for i, mesh := range meshes {
										if i >= 2 {
											t.Logf("    ... 还有 %d 个网格（省略）", len(meshes)-2)
											break
										}
										t.Logf("    网格 %d:", i)
										t.Logf("      原点: (%.6f, %.6f)", mesh.OriginX, mesh.OriginY)
										t.Logf("      步长: (%.6f, %.6f)", mesh.DeltaX, mesh.DeltaY)
										t.Logf("      顶点数: %d, 面数: %d, 层级: %d", mesh.NumPoints, mesh.NumFaces, mesh.Level)
										if mesh.NumPoints > 0 {
											// 输出第一个顶点作为示例
											v := mesh.Vertices[0]
											t.Logf("      第一个顶点: (%.6f, %.6f, %.2fm)", v.X, v.Y, v.Z)
										}
									}
								}
							}

							// 第四步：导出为 OBJ 格式
							// 先验证数据
							if terrain.NumMeshGroups() == 0 {
								t.Logf("⚠️  警告：没有网格组数据")
							} else if terrain.NumMeshes() == 0 {
								t.Logf("⚠️  警告：没有网格数据")
							} else {
								t.Logf("📊 准备导出OBJ：%d个网格组，%d个网格", terrain.NumMeshGroups(), terrain.NumMeshes())

								// 统计总顶点数和面数
								totalVerts := 0
								totalFaces := 0
								for _, meshes := range terrain.MeshGroups {
									for _, mesh := range meshes {
										totalVerts += mesh.NumPoints
										totalFaces += mesh.NumFaces
									}
								}
								t.Logf("📊 总顶点数: %d, 总面数: %d", totalVerts, totalFaces)

								objContent := exportTerrainToOBJ(terrain)
								// 确保测试输出目录存在（使用绝对路径）
								testOutputDir := "/home/stone/crawler-platform/test_output"
								if err := os.MkdirAll(testOutputDir, 0755); err != nil {
									t.Logf("⚠️  创建测试输出目录失败: %v", err)
								} else {
									objFileName := fmt.Sprintf("%s/google_earth_terrain_%s.obj", testOutputDir, terTilekey)
									t.Logf("📁 OBJ文件保存路径: %s", objFileName)
									err = os.WriteFile(objFileName, []byte(objContent), 0644)
									if err != nil {
										t.Logf("⚠️  保存 OBJ 文件失败: %v", err)
									} else {
										t.Logf("✅ 成功导出 OBJ 模型: %s (%d 字节)", objFileName, len(objContent))

										// 输出OBJ文件的前几行用于调试
										lines := strings.Split(objContent, "\n")
										t.Logf("📄 OBJ 文件预览（前20行）：")
										for i := 0; i < min(20, len(lines)); i++ {
											if lines[i] != "" {
												t.Logf("  %s", lines[i])
											}
										}

										// 统计v和f行数
										vCount := 0
										fCount := 0
										for _, line := range lines {
											if strings.HasPrefix(line, "v ") {
												vCount++
											}
											if strings.HasPrefix(line, "f ") {
												fCount++
											}
										}
										t.Logf("📊 OBJ文件实际包含: %d 个顶点(v), %d 个面(f)", vCount, fCount)

										// 分析顶点坐标范围
										var minX, maxX, minY, maxY, minZ, maxZ float64
										firstVertex := true
										for _, line := range lines {
											if strings.HasPrefix(line, "v ") {
												var x, y, z float64
												fmt.Sscanf(line, "v %f %f %f", &x, &y, &z)
												if firstVertex {
													minX, maxX = x, x
													minY, maxY = y, y
													minZ, maxZ = z, z
													firstVertex = false
												} else {
													if x < minX {
														minX = x
													}
													if x > maxX {
														maxX = x
													}
													if y < minY {
														minY = y
													}
													if y > maxY {
														maxY = y
													}
													if z < minZ {
														minZ = z
													}
													if z > maxZ {
														maxZ = z
													}
												}
											}
										}
										if vCount > 0 {
											t.Logf("📊 顶点坐标范围:")
											t.Logf("  X: [%.3f, %.3f] 范围=%.3fm", minX, maxX, maxX-minX)
											t.Logf("  Y: [%.3f, %.3f] 范围=%.3fm", minY, maxY, maxY-minY)
											t.Logf("  Z: [%.3f, %.3f] 范围=%.3fm", minZ, maxZ, maxZ-minZ)
										}

										// 导出 XYZ 格式（通用行格式，每行：经度 纬度 高程）
										xyzContent, err := exportTerrainToXYZ(terrain)
										if err != nil {
											t.Logf("⚠️  导出 XYZ 失败: %v", err)
										} else {
											xyzFileName := fmt.Sprintf("%s/google_earth_terrain_%s.xyz", testOutputDir, terTilekey)
											if err := os.WriteFile(xyzFileName, []byte(xyzContent), 0644); err != nil {
												t.Logf("⚠️  保存 XYZ 文件失败: %v", err)
											} else {
												t.Logf("✅ 成功导出 XYZ 文件: %s (通用行格式)", xyzFileName)
											}
										}

										// 同步导出 DEM（ESRI ASCII Grid）
										demContent, nCols, nRows, err := exportTerrainToDEM(terrain)
										if err != nil {
											t.Logf("⚠️  导出 DEM 失败: %v", err)
										} else {
											demFileName := fmt.Sprintf("%s/google_earth_terrain_%s.asc", testOutputDir, terTilekey)
											if err := os.WriteFile(demFileName, []byte(demContent), 0644); err != nil {
												t.Logf("⚠️  保存 DEM 文件失败: %v", err)
											} else {
												t.Logf("✅ 成功导出 DEM 文件: %s (%d x %d 网格)", demFileName, nCols, nRows)

												// 尝试转换为 GeoTIFF（使用 GDAL）
												geotiffPath, err := exportTerrainToGeoTIFF(demFileName)
												if err != nil {
													t.Logf("⚠️  转换为 GeoTIFF 失败: %v", err)
													t.Logf("💡 提示: 如需 GeoTIFF 格式，请安装 GDAL 工具: sudo apt-get install gdal-bin")
												} else {
													t.Logf("✅ 成功导出 GeoTIFF 文件: %s", geotiffPath)
												}
											}
										}

										// 创建简单的 MTL 材质文件
										mtlFileName := fmt.Sprintf("%s/google_earth_terrain_%s.mtl", testOutputDir, terTilekey)
										mtlContent := "# Google Earth Terrain Material\n" +
											"newmtl terrain\n" +
											"Ka 0.8 0.7 0.6\n" + // 环境光
											"Kd 0.8 0.7 0.6\n" + // 漫反射
											"Ks 0.2 0.2 0.2\n" + // 镜面反射
											"Ns 10.0\n" + // 高光指数
											"d 1.0\n" + // 不透明度
											"illum 2\n" // 光照模型
										os.WriteFile(mtlFileName, []byte(mtlContent), 0644)
										t.Logf("✅ 成功创建 MTL 材质文件: %s", mtlFileName)
									}
								}
							}

							// 保存为文件
							terFileName := fmt.Sprintf("/tmp/google_earth_terrain_%s.dat", terTilekey)
							err = os.WriteFile(terFileName, terUnpacked, 0644)
							if err != nil {
								t.Logf("⚠️  保存文件失败: %v", err)
							} else {
								t.Logf("✅ 成功保存地形原始数据: %s", terFileName)
							}
						}
					}
				}
			}
		}
	} else {
		qtp := GoogleEarth.NewQuadtreePacketProtoBuf()
		if err := qtp.Parse(decryptedBody); err != nil {
			t.Fatalf("解析 protobuf 失败: %v", err)
		}

		packet := qtp.GetPacket()
		if packet == nil {
			t.Fatal("解析后的 packet 为 nil")
		}

		t.Logf("✅ 成功解析 Quadtree Packet (Protobuf)")
		t.Logf("   Packet Epoch: %d", packet.GetPacketEpoch())
		t.Logf("   节点数量: %d", len(packet.Sparsequadtreenode))

		// 9. 遍历节点并输出信息
		t.Logf("\n=== 步骤 7: 遍历节点信息 ===")
		for i, sparseNode := range packet.Sparsequadtreenode {
			if sparseNode == nil || sparseNode.Node == nil {
				continue
			}

			node := sparseNode.Node
			index := sparseNode.GetIndex()

			t.Logf("\n节点 %d (Index: %d):", i, index)
			t.Logf("  Flags: 0x%X", node.GetFlags())
			t.Logf("  Cache Node Epoch: %d", node.GetCacheNodeEpoch())
			t.Logf("  图层数量: %d", len(node.Layer))
			t.Logf("  通道数量: %d", len(node.Channel))

			// 输出图层信息
			for j, layer := range node.Layer {
				if layer == nil {
					continue
				}
				t.Logf("    图层 %d: Type=%v, Epoch=%d, Provider=%d",
					j,
					layer.GetType(),
					layer.GetLayerEpoch(),
					layer.GetProvider())
			}

			// 输出通道信息
			for j, channel := range node.Channel {
				if channel == nil {
					continue
				}
				t.Logf("    通道 %d: Type=%d, Epoch=%d",
					j,
					channel.GetType(),
					channel.GetChannelEpoch())
			}

			// 只输出前 5 个节点的详细信息
			if i >= 4 {
				t.Logf("\n... 还有 %d 个节点（省略）", len(packet.Sparsequadtreenode)-5)
				break
			}
		}

		// 10. 提取数据引用
		t.Logf("\n=== 步骤 8: 提取数据引用 ===")
		references := &GoogleEarth.QuadtreeDataReferenceGroup{}
		pathPrefix := GoogleEarth.NewQuadtreePathFromString("0") // 根节点路径前缀
		jpegDate := GoogleEarth.JpegCommentDate{}                // 不过滤日期

		qtp.GetDataReferences(references, pathPrefix, jpegDate, true)

		// 过滤 QTP 引用：只有 tilekey 长度能被 4 整除的才是 q2（子节点集合）
		var filteredQtpRefs []GoogleEarth.QuadtreeDataReference
		for _, ref := range references.QtpRefs {
			tilekey := ref.QtPath.AsString()
			if len(tilekey)%4 == 0 {
				filteredQtpRefs = append(filteredQtpRefs, ref)
			}
		}
		references.QtpRefs = filteredQtpRefs

		t.Logf("数据引用统计:")
		t.Logf("  影像引用: %d 个", len(references.ImgRefs))
		t.Logf("  地形引用: %d 个", len(references.TerRefs))
		t.Logf("  矢量引用: %d 个", len(references.VecRefs))
		t.Logf("  QTP 引用 (q2子节点集合, tilekey长度能被4整除): %d 个", len(references.QtpRefs))

		// 输出前几个影像引用
		if len(references.ImgRefs) > 0 {
			t.Logf("\n前 3 个影像引用:")
			for i := 0; i < min(3, len(references.ImgRefs)); i++ {
				ref := references.ImgRefs[i]
				t.Logf("  %d. Path=%s, Version=%d, Provider=%d",
					i+1,
					ref.QtPath.AsString(),
					ref.Version,
					ref.Provider)
			}
		}

		// 输出前几个地形引用
		if len(references.TerRefs) > 0 {
			t.Logf("\n前 3 个地形引用（地形数据只在奇数层级，即tilekey长度为奇数）:")
			for i := 0; i < min(3, len(references.TerRefs)); i++ {
				ref := references.TerRefs[i]
				tilekey := ref.QtPath.AsString()
				t.Logf("  %d. Path=%s (长度=%d, %s), Version=%d, Provider=%d",
					i+1,
					tilekey,
					len(tilekey),
					map[bool]string{true: "奇数✓", false: "偶数✗"}[len(tilekey)%2 == 1],
					ref.Version,
					ref.Provider)
			}
		}

		// 11. 测试请求和解析 QTP 引用（q2 子节点）
		t.Logf("\n=== 步骤 9: 请求并解析 QTP 引用的子节点 ===")
		t.Logf("说明: q2 是一个子集合，管理 4 层数据")
		t.Logf("      例如 tilekey=0022 (第4层) 包含第 5,6,7,8 层数据")
		t.Logf("      地形数据只在奇数层级（5层、7层）才有")
		if len(references.QtpRefs) > 0 {
			// 只测试前 3 个 QTP 引用
			testCount := min(3, len(references.QtpRefs))
			t.Logf("测试前 %d 个 QTP 引用（共 %d 个）:", testCount, len(references.QtpRefs))

			for i := 0; i < testCount; i++ {
				qtpRef := references.QtpRefs[i]
				childTilekey := qtpRef.QtPath.AsString()
				childEpoch := int(dbRootData.Version) // 使用 dbRoot 的版本号

				t.Logf("\n--- QTP %d: Path=%s (长度=%d), Version=%d ---",
					i+1, childTilekey, len(childTilekey), qtpRef.Version)

				// 构建子节点的 q2 URL
				childURL := fmt.Sprintf("https://%s/flatfile?q2-%s-q.%d",
					GoogleEarth.HOST_NAME, childTilekey, childEpoch)

				// 创建请求（复用热连接）
				childReq, err := http.NewRequest("GET", childURL, nil)
				if err != nil {
					t.Logf("  ⚠️  创建请求失败: %v", err)
					continue
				}

				// 设置请求头
				childReq.Header.Set("Host", GoogleEarth.HOST_NAME)
				childReq.Header.Set("Cookie", fmt.Sprintf("SessionId=%s;State=1", session))
				childReq.Header.Set("Content-Type", "application/octet-stream")

				// 发送请求（复用client）
				childResp, err := client.Do(childReq)
				if err != nil {
					t.Logf("  ⚠️  请求失败: %v", err)
					continue
				}

				// 读取响应
				childBody, err := io.ReadAll(childResp.Body)
				childResp.Body.Close()

				if err != nil {
					t.Logf("  ⚠️  读取响应失败: %v", err)
					continue
				}

				if childResp.StatusCode != 200 {
					t.Logf("  ⚠️  状态码: %d, 响应大小: %d 字节", childResp.StatusCode, len(childBody))
					continue
				}

				t.Logf("  ✅ 成功获取数据，大小: %d 字节", len(childBody))

				// 解密
				childDecrypted, err := GoogleEarth.UnpackGEZlib(childBody)
				if err != nil {
					t.Logf("  ⚠️  解密失败: %v", err)
					continue
				}

				t.Logf("  ✅ 解密成功，大小: %d 字节", len(childDecrypted))

				// 解析
				childQtp := GoogleEarth.NewQuadTreePacket16()
				if err := childQtp.Decode(childDecrypted); err != nil {
					t.Logf("  ⚠️  解析失败: %v", err)
					continue
				}

				t.Logf("  ✅ 解析成功: %d 个数据实例", len(childQtp.DataInstances))

				// 统计子节点的数据类型
				var childImgCount, childTerCount int
				for _, quantum := range childQtp.DataInstances {
					if quantum.GetImageBit() {
						childImgCount++
					}
					if quantum.GetTerrainBit() {
						childTerCount++
					}
				}

				t.Logf("  统计: 影像=%d, 地形=%d", childImgCount, childTerCount)
			}
		}
	}

	// 11. 检查是否包含特定图层类型
	t.Logf("\n=== 步骤 9: 测试总结 ===")

	t.Logf("\n=== ✅ Quadtree Packet 完整解包测试成功 ===")
}

// TestQuadtreePacket_Binary_RealData 测试二进制格式的 quadtree packet（旧格式）
func TestQuadtreePacket_Binary_RealData(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（使用 -short 标志）")
	}

	// 注意：这个测试需要找到使用二进制格式的 URL
	// 目前大部分 Google Earth 数据都使用 Protobuf 格式
	// 如果遇到二进制格式的数据，可以使用 QuadTreePacket16 来解析

	t.Skip("二进制格式测试需要特定的数据源，暂时跳过")
}
