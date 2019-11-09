### 所用技术
Golang+Mysql+Redis+NSQ完成,Redis主要用于对设备和服务器长链接建立的对应关系，NSQ用于logic层服务器和connect层服务器的异步通信，web框架使用了gin(对gin进行了简单的封装)，日志框架使用了zap,TCP拆包粘包，唯一消息id生成器，数据库统一事务管理等。
### 项目分层设计
项目主要分为两层，connect层和logic层，public包下放置了一些connect层和logic层公用的代码和一些基础库。  
connect连接层，主要维护和客户端的tcp连接，所以connect是有状态的，connect包含了TCP拆包粘包，消息解码，客户端心跳等一些逻辑。    
logic逻辑层是无状态的，主要维护消息的转发逻辑，以及对外提供http接口，提供一些聊天系统基本的业务功能，例如，登录，注册，添加好友，删除好友，创建群组，添加人到群组，群组踢人等功能
### logic层和connect层的通信
服务器通信的原则是能异步就使用异步通信，不能异步就是用RPC。  
通信中，设备登录使用RPC进行通信（简单使用golang原生RPC实现，暂时还没有做服务注册发现），其它的都使用NSQ进行异步通信。 
### 如何保证消息不丢不重
首先我们要搞清楚什么情况下消息会丢会重  
消息重复：客户端发送消息，由于网络原因或者其他原因，客户端没有收到服务器的发送消息回执，这时客户端会重复发送一次，假设这种情况，客户端发送的消息服务器已经处理了，但是回执消息丢失了，客户端再次发送，服务器就会重复处理，这样，接收方就会看到两条重复的消息，解决这个问题的方法是，客户端对发送的消息增加一个递增的序列号，服务器保存客户端发送的最大序列号，当客户端发送的消息序列号大于序列号，服务器正常处理这个消息，否则，不处理。  
消息丢失：服务器投递消息后，如果客户端网络不好，或者已经断开连接，或者是已经切换到其他网络，服务器并不是立马可以感知到的，服务器只能感知到消息已经投递出去了，所以这个时候，就会造成消息丢失。  
怎样解决：  
1:消息持久化：  
2:投递消息增加序列号  
3:增加消息投递的ACK机制，就是客户端收到消息之后，需要回执服务器，自己收到了消息。  
这样，服务器就可以知道，客户端那些消息已经收到了，当客户端检测到TCP不可用（使用心跳机制），重新建立TCP连接，带上自己收到的消息的最大序列号，触发一次消息同步，服务器根据这个序列号，将客户端未收到再次投递给客户端，这样就可以保证消息不丢失了。  
### 消息转发逻辑，是采用读扩散还是写扩散
首先解释一下，什么是读扩散，什么是写扩散  
读扩散：当两个客户端A,B发消息时，首先建立一个会话，A,B发所发的每条消息都会写到这个会话里，消息同步的时候，只需要查询这个会话即可。群发消息也是一样，群组内部发送消息时，也是先建立一个会话，都将这个消息写入这个会话中，触发消息同步时，只需要查询这个会话就行，读扩散的好处就是，每个消息只需要写入数据库一次就行，但是，每个会话都要维持一个消息序列，作消息同步，客户端要上传N个（用户会话个数）序列号，服务器要为这N个序列号做消息同步，想想一下，成百上千个序列号，是不是有点恐怖。  
写扩散：就是每个用户维持一个消息列表，当有其他用户给这个用户发送消息时，给这个用户的消息列表插入一条消息即可，这样做的好处就是，每个用户只需要维护一个消息列表，也只需要维护一组序列号，坏处就是，一个群组有多少人，就要插入多少条消息，DB的压力会增大。  
如何选型呢，我采用的时写扩散，据说，微信3000人的群也是这样干的。当然也有一些IM，对大群做了特殊处理，对超大群采用读扩散的模式。
### 消息拆包
首先说一下，一个消息是怎样定义的，使用的TLV,解释一下：  
T:消息类型  
L:消息长度  
V:实际的数据  
我是怎样做的，消息也是采用TLV的格式，T采用了两个字节，L:也是两个字节，两个字节最大可以存储65536的无符号整形，所以的单条数据的长度不能超过65536，再大，就要考虑分包了。  
拆包具体是怎样实现的，首先，我自己实现了一个buffer,这个buffer实质上就是一个slice,首先，先从系统缓存里面读入到这个buffer里面，然后的拆包的所有处理，都在这个buffer里面完成，拆出来的字节数组，也是复用buffer的内存，拆出来的数组要是连续的，怎样保证呢，我是这样做的。  
每次从系统缓存读取数据放到buffer后，然后进行拆包，当拆到不能再拆的时候（buffer里面不足一个消息），把buffer里面的剩余字节迁移到slice的头部，下一次的从系统缓存的读到的字节，追加到后面就行了，这样就能保证所拆出来的消息时连续的字节数组。  
消息协议使用Google的Protocol   Buffers,具体消息协议定制在/public/proto包下
### 心跳保活

### 单用户多设备支持
首先说一下，单用户单设备的消息转发是怎样的，使用写扩散的方式，比如有两个用户A和B聊天，当A用户给B用户发消息时，需要给B用户的消息列表插入一条消息，然后，通过TCP长链接，将消息投递给B就行。  
现在升级到但用户多设备，举个例子，A用户下有a1,a2两个设备同时在线，B用户有b1,b2两个设备同时在线，A用户a1设备给B用户发送消息，消息转发流程就会变成这样：  
1:给B用户的消息列表插入一条消息，然后将消息投递给设备b1,b2（这里注意，一个用户不管有多少设备在线，指维护一个消息列表）  
2:给A用户的消息列表插入一条消息，然后将消息投递给设备a1,a2（这里注意，虽然a1是发送消息者，但是还会收到消息投递，当设备发现发送消息的设备id是自己的，就不对消息做处理，只同步序列号就行）  
这里看到，单用户单设备模式下，一条消息只需要存储一份，而在单用户多设备模式下，一条消息需要存储两份

### 消息唯一id
唯一消息id的主要作用是用来标识一次消息发送的完整流程，消息发送->消息投递->消息投递回执，用来线上排查线上问题。  
每一个消息有唯一的消息的id，由于消息发送频率比较高，所以性能就很重要，当时没有找到合适第三方库，所以就自己实现了一个，原理就是，每次从数据库中拿一个数据段，用完了再去数据库拿，当用完之后去从数据库拿的时候，会有一小会的阻塞，为了解决这个问题，就做成了异步的，就是保证内存中有n个可用的id，当id消耗掉小于n个时，就从数据库获取生成，当达到n个时，goroutine阻塞等待id被消耗，如此往复。

### 主要逻辑
client: 客户端  
connect:连接层  
logic:逻辑层  
mysql:存储层  

#### 登录
[![3496be2f9ee9d33e.jpg](http://www.wailian.work/images/2018/11/12/3496be2f9ee9d33e.jpg)](http://www.wailian.work/image/BVGV24)

#### 单发
[![00d7e21cccc9050e.jpg](http://www.wailian.work/images/2018/11/12/00d7e21cccc9050e.jpg)](http://www.wailian.work/image/BVGZkp)
#### 群发
[![7ee3ada2baf1dec0.jpg](http://www.wailian.work/images/2018/11/12/7ee3ada2baf1dec0.jpg)](http://www.wailian.work/image/BVGtLc)

### api文档
https://documenter.getpostman.com/view/4164957/RzZ4q2hJ

### 消息协议
所有的消息都封装为connect包下的:

    Package struct {
        Code    int    // 消息类型
        Content []byte // 消息体
    }      
其中的 code为:

     const (
     	CodeSignIn         = 1 // 设备登录
     	CodeSignInACK      = 2 // 设备登录回执
     	CodeSyncTrigger    = 3 // 消息同步触发
     	CodeHeadbeat       = 4 // 心跳
     	CodeHeadbeatACK    = 5 // 心跳回执
     	CodeMessageSend    = 6 // 消息发送
     	CodeMessageSendACK = 7 // 消息发送回执
     	CodeMessage        = 8 // 消息投递
     	CodeMessageACK     = 9 // 消息投递回执
     )      
content为protobuff对象生成的二进制数据，主要数据模型如下:           
1. 设备登录:


    pb.SignIn{
        DeviceId: 设备id,
        UserId:   用户id,
        Token:    设备token，为uuid,设备唯一标志,
    }
2. 消息同步:


    pb.SyncTrigger{
         SyncSequence: 同步序列号
    }
3.  发送消息


    type MessageSend struct {
    	ReceiverType int32  `protobuf:"varint,1,opt,name=receiver_type,json=receiverType" json:"receiver_type,omitempty"` 接收者类型 1-个人消息、2-群组消息
    	ReceiverId   int64  `protobuf:"varint,2,opt,name=receiver_id,json=receiverId" json:"receiver_id,omitempty"`  接收者id
    	Type         int32  `protobuf:"varint,3,opt,name=type" json:"type,omitempty"`    消息类型 文本、图片、语音
    	Content      string `protobuf:"bytes,4,opt,name=content" json:"content,omitempty"`   //消息内容
    	SendSequence int64  `protobuf:"varint,5,opt,name=send_sequence,json=sendSequence" json:"send_sequence,omitempty"` 消息序列号
    	SendTime     int64  `protobuf:"varint,6,opt,name=send_time,json=sendTime" json:"send_time,omitempty"`   发送时间
    } 