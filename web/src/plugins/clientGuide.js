// 客户端连接指引默认内容（门户未配置时回退）。
// steps 为 HTML 片段，支持 {{server_addr}} 占位符，前端渲染时替换为用户实际服务器地址。
export const defaultClientGuide = [
  {
    name: "Windows",
    steps: [
      "下载并安装 Cisco AnyConnect 客户端",
      '打开客户端，在地址栏输入：<code class="addr-code">{{server_addr}}</code>',
      '点击"连接"，输入用户名和密码即可',
    ],
  },
  {
    name: "macOS",
    steps: [
      "从 App Store 下载 Cisco AnyConnect",
      '打开后输入服务器地址：<code class="addr-code">{{server_addr}}</code>',
      "输入用户名和密码，点击 Connect",
    ],
  },
  {
    name: "Linux",
    steps: [
      '安装 OpenConnect：<code class="addr-code">apt install openconnect</code> 或 <code class="addr-code">yum install openconnect</code>',
      '命令行连接：<code class="addr-code">sudo openconnect {{server_addr}}</code>',
      "按提示输入用户名和密码",
    ],
  },
  {
    name: "Android",
    steps: [
      "从 Google Play 或应用商店下载 Cisco AnyConnect",
      '添加新连接，输入服务器地址：<code class="addr-code">{{server_addr}}</code>',
      "输入用户名和密码连接",
    ],
  },
  {
    name: "iOS",
    steps: [
      "从 App Store 下载 Cisco AnyConnect",
      '打开后点击"添加 VPN 连接"，输入服务器：<code class="addr-code">{{server_addr}}</code>',
      "保存后点击连接，输入用户名和密码",
    ],
  },
];
