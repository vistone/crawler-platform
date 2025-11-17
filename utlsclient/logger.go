package utlsclient

// 该文件已迁移至项目级 logger 包，保留向后兼容的转发（如需可删除）
import "crawler-platform/logger"

func SetGlobalLogger(l logger.Logger) { logger.SetGlobalLogger(l) }
func GetGlobalLogger() logger.Logger  { return logger.GetGlobalLogger() }
func InitGlobalLogger(l logger.Logger) { logger.InitGlobalLogger(l) }
func Debug(format string, args ...interface{}) { logger.Debug(format, args...) }
func Info(format string, args ...interface{})  { logger.Info(format, args...) }
func Warn(format string, args ...interface{})  { logger.Warn(format, args...) }
func Error(format string, args ...interface{}) { logger.Error(format, args...) }
