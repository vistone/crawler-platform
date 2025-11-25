package GoogleEarth

// 数据库名称常量
const (
	EARTH = "earth"
	MARS  = "mars"
	MOON  = "moon"
	SKY   = "sky"
	TM    = "tm" //这个是历史卫星影像数据
)

// 数据格式魔法数字
const (
	CRYPTED_JPEG_MAGIC         = 0xA6EF9107
	CRYPTED_MODEL_DATA_MAGIC   = 0x487B
	CRYPTED_ZLIB_MAGIC         = 0x32789755
	DECRYPTED_MODEL_DATA_MAGIC = 0x0183
	DECRYPTED_ZLIB_MAGIC       = 0x7468DEAD
)

const (
	HOST_NAME            = "kh.google.com"
	TM_HOST_NAME         = "khmdb.google.com"
	DBROOT_PATH          = "/dbRoot.v5"
	DBROOT_WITH_DB_PATH  = "/dbRoot.v5?db=%s"              // 带数据库名称的 dbRoot 路径,mars,moon,sky, tm是历史数据
	Q2_PATH              = "/flatfile?q2-%s-q.%d"          //%s 是tilekey，%d 是epoch
	QPQ2_PATH            = "/flatfile?db=%s&qp-%s-q.%d"    //带数据库名称的q2数据 第一个 %s 是tm，mars,moon,sky,第二个%s这里是tilekey，%d 是Epoch
	IMAGERY_PATH         = "/flatfile?f1-%s-i.%d"          // 带数据库名称的imagery数据，%s是tilekey，%d是imageryEpoch
	IMAGERY_WITH_TM_PATH = "/flatfile?db=tm&f1-%s-i.%d-%s" //历史卫星影像数据，%s是tilekey，%d是imageryEpoch，%s 日期
	Terrain_PATH         = "/flatfile?f1c-%s-t.%d"         // 带数据库名称的terrain数据，%s是tilekey，%d是terrainEpoch
)

/*
这里是google earth的tilekey编号规则
		   c0    c1
		|-----|-----|
	r1	|  3  |  2  |
		|-----|-----|
	r0	|  0  |  1  |
		|-----|-----|

q2 是一个数据集合 只能是tilekey的长度能被4整除的层级才可以当集合
- 节点有两种编号方案：

- 1) “子索引（Subindex）”。这种编号从树的顶部开始，对每一层按从左到右进行，如下所示：

                      0
                   /     \                           .
                 1  86 171 256
              /     \                                .
            2  3  4  5 ...
          /   \                                      .
         6  7  8  9  ...

- 注意第二行比较奇怪，并不是从左到右的顺序。然而，在 Keyhole 中，根节点是特殊的：它不采用这种奇怪的排序。它看起来像这样：

                      0
                   /     \                           .
                 1  2  3  4
              /     \                                .
            5  6  7  8 ...
         /     \                                     .
       21 22 23 24  ...

- 第二行的这种“错乱排序（mangling）”由构造函数的一个参数控制。
*/
