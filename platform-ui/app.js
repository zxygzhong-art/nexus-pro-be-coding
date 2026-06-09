/* =========================================================
   Nexus OA Platform — App logic
   ========================================================= */

// ---------- Sample data ----------
const ASSISTANTS = [
  { id: 'employee-care',   emoji: '🙋', title: '員工疑難雜症助理', desc: '針對職場倦怠、人際衝突或辦公室各種小問題提供匿名諮詢，並能引導至正確的申訴或 IT 報修管道。', tag: 'workflow' },
  { id: 'sales-analytics', emoji: '📈', title: '業務報表分析',     desc: '深度解讀銷售數據，自動抓取異常衰退或成長的區域，並根據歷史資料預測下季度營收目標。',        tag: 'analytics' },
  { id: 'product-catalog', emoji: '📖', title: '產品目錄助理',     desc: '即時查詢上萬種產品規格，提供跨型號功能對比，並能根據客戶需求一鍵生成簡化版的產品提案書。',    tag: 'doc' },
  { id: 'recruiting',      emoji: '🤝', title: '招聘獵頭助理',     desc: '自動產生職位說明（JD）、篩選海量履歷並給出匹配分數，還能自動發送面試邀約信。',              tag: 'workflow' },
  { id: 'training-mentor', emoji: '🎓', title: '培訓學長姐',       desc: '為新進員工量身訂製入職訓計畫，根據崗位需求推薦學習清單，並即時解答公司內部的作業規範。',    tag: 'doc' },
  { id: 'legal-contract',  emoji: '⚖️', title: '合約法務顧問',     desc: '自動掃描合約中的潛在風險條款，檢查是否符合公司合規標準，並比對不同版本間的文字差異。',      tag: 'doc' },
  { id: 'project-manager', emoji: '🏗️', title: '專案進度管家',     desc: '自動彙整各專案里程碑，當進度落後或資源衝突時發出預警，並產出易讀的專案進度週報。',          tag: 'workflow' },
  { id: 'procurement',     emoji: '🛒', title: '採購供應專員',     desc: '自動對比供應商報價、追蹤採購單發貨進度，並在安全庫存量過低時自動發出採購補貨提醒。',        tag: 'workflow' },
  { id: 'marketing',       emoji: '📢', title: '品牌行銷大師',     desc: '快速產生社群貼文文案、產品 Slogan 以及節慶活動促銷方案，支援多種語氣切換以適應不同平台。',  tag: 'doc' },
  { id: 'crm',             emoji: '💬', title: '客戶關係維繫專家', desc: '提醒重要客戶的續約期或生日，建議最佳跟進話術，並自動彙整 CRM 系統中的客戶回饋與不滿。',     tag: 'workflow' },
  { id: 'esg',             emoji: '🌍', title: 'ESG 永續助理',     desc: '追蹤企業碳足跡數據，自動生成永續報告書草稿，並根據法規變化提醒公司進行節能減碳調整。',      tag: 'analytics' },
  { id: 'security',        emoji: '🛡️', title: '資安風控官',       desc: '監控內部帳號異常登入行為，自動提醒員工定期更換密碼，並提供最新的社交工程詐騙預防宣導。',    tag: 'it' },
];

const FORM_COLUMNS = [
  {
    title: '人事考勤類', emoji: '👥',
    items: [
      { emoji: '🗓️', title: '請假申請單',         desc: '特休 / 事假 / 病假 / 公假' },
      { emoji: '⏰', title: '加班核准申請單',     desc: '平日延時、假日加班' },
      { emoji: '⏰', title: '加班申請單',         desc: '依加班核准單實際加班時填寫' },
      { emoji: '🕒', title: 'HR-005 補卡單',     desc: '漏打卡或打卡異常補登' },
      { emoji: '👤', title: 'HR-009 外勤請假單', desc: '外勤、客戶拜訪等請假' },
    ]
  },
  {
    title: '財會相關', emoji: '💰',
    items: [
      { emoji: '💸', title: '費用報支申請單',     desc: '日常費用核銷' },
      { emoji: '💳', title: '員工代墊款請領清單', desc: '員工代墊費用請領' },
      { emoji: '💴', title: '零用金請款單',       desc: '小額零用金核銷' },
      { emoji: '💵', title: '預支款申請單',       desc: '出差或專案預支款' },
      { emoji: '✅', title: '費用核准申請單',     desc: '費用事前核准' },
    ]
  },
];

const DRAFTS = [
  { emoji: '🗓️', title: '請假申請單',   summary: '4/22 特休一天 · 陪同家人就醫', updated: '今天 11:42' },
];

const FORM_CATEGORIES = [
  {
    title: '人事考勤類', icon: '👥',
    items: [
      { emoji: '🗓️', title: '請假申請單',         desc: '特休 / 事假 / 病假 / 公假' },
      { emoji: '⏰', title: '加班核准申請單',     desc: '平日延時、假日加班皆可使用' },
      { emoji: '⏰', title: '加班申請單',         desc: '依加班核准單實際加班時填寫' },
      { emoji: '👤', title: 'HR-009 外勤請假單', desc: '外勤、客戶拜訪等請假' },
      { emoji: '🕒', title: 'HR-005 補卡單',     desc: '漏打卡或打卡異常補登' },
      { emoji: '⏰', title: 'HR-004 銷加班單',   desc: '取消已核准加班申請' },
      { emoji: '🗓️', title: 'HR-003 銷假單',     desc: '取消已核准請假申請' },
    ]
  },
  {
    title: '人資相關', icon: '👥',
    items: [
      { emoji: '📋', title: '人事/職務/薪資異動單',       desc: '異動職務、調薪、調動' },
      { emoji: '➕', title: 'iKala 人員增補申請單',       desc: '新增職缺與招募' },
      { emoji: '🔐', title: '新人報到權限開通單',         desc: '新人系統權限與帳號開通' },
      { emoji: '🔁', title: '復職申請單',                 desc: '留停或留職停薪後復職' },
      { emoji: '⏸️', title: '留職停薪申請單',             desc: '育嬰、進修、家庭因素' },
      { emoji: '👋', title: '離職及退休申請單',           desc: '離職、退休手續辦理' },
      { emoji: '🔄', title: '離職(留職停薪)交接申請單',   desc: '工作交接清單與確認' },
      { emoji: '🎓', title: '外部教育訓練申請單',         desc: '參加外部課程、研討會' },
      { emoji: '🏆', title: 'iKala Peer Bonus提名表',     desc: '同事提名表揚' },
    ]
  },
  {
    title: '財會相關', icon: '💰',
    items: [
      { emoji: '💰', title: '個別呆帳申請單',                       desc: '應收帳款轉呆帳' },
      { emoji: '💵', title: '預支款報銷及繳回單',                   desc: '預支款核銷或繳回' },
      { emoji: '💵', title: '預支款申請單',                         desc: '出差或專案預支款' },
      { emoji: '🧾', title: '銷貨/進貨退回及折讓(作廢發票)申請單',  desc: '退回、折讓、作廢處理' },
      { emoji: '💳', title: '員工代墊款請領清單',                   desc: '員工代墊費用請領' },
      { emoji: '💴', title: '零用金請款單',                         desc: '小額零用金核銷' },
      { emoji: '💸', title: '費用報支申請單',                       desc: '日常費用核銷' },
      { emoji: '💳', title: '預付款及刷卡授權單',                   desc: '預付款或刷卡授權' },
      { emoji: '✅', title: '費用核准申請單',                       desc: '費用事前核准' },
    ]
  },
  {
    title: '採購相關', icon: '🛒',
    items: [
      { emoji: '🧾', title: '請款報銷單',         desc: '採購完成請款' },
      { emoji: '✅', title: '驗收單',             desc: '商品/服務驗收確認' },
      { emoji: '💳', title: '預付款及刷卡授權單', desc: '採購預付款授權' },
      { emoji: '🛒', title: '採購單-流量型',      desc: '流量類採購' },
      { emoji: '🛒', title: '採購單',             desc: '一般採購' },
      { emoji: '📝', title: '請購單',             desc: '採購需求提出' },
      { emoji: '⭐', title: '供應商評鑑表',       desc: '供應商定期評鑑' },
      { emoji: '🏢', title: '供應商基本資料表',   desc: '新供應商建檔' },
    ]
  },
  {
    title: '銷售相關', icon: '📈',
    items: [
      { emoji: '🤝', title: 'BP異動申請單',               desc: 'Business Partner 異動' },
      { emoji: '📊', title: 'B2B報價核准申請單',          desc: 'B2B 案件報價核准' },
      { emoji: '📊', title: 'EDP報價核准申請單',          desc: 'EDP 案件報價核准' },
      { emoji: '☁️', title: 'Cloud Team- Cloud Credit申請單', desc: 'Cloud Credit 申請' },
      { emoji: '📈', title: 'CLOUD毛利異動單',            desc: 'Cloud 案件毛利調整' },
      { emoji: '📣', title: '客訴單',                     desc: '客戶投訴受理' },
    ]
  },
  {
    title: '合約相關', icon: '📑',
    items: [
      { emoji: '📑', title: '合約/一般文件草擬申請單', desc: '請法務協助草擬' },
      { emoji: '📂', title: '合約歸檔申請單',         desc: '已簽合約歸檔' },
      { emoji: '🖋️', title: '合約用印申請單',         desc: '合約用印申請' },
      { emoji: '🔍', title: '合約審閱申請單',         desc: '請法務協助審閱' },
    ]
  },
  {
    title: '行政相關', icon: '🧾',
    items: [
      { emoji: '✈️', title: '國內外出差旅費報告表', desc: '出差返國後核銷' },
      { emoji: '🛒', title: '出差採購單',           desc: '出差期間採購' },
      { emoji: '🛫', title: '國內外出差申請表',     desc: '出差行程預先申請' },
      { emoji: '💸', title: '費用報支申請單',       desc: '行政類費用核銷' },
      { emoji: '💳', title: '預付款及刷卡授權單',   desc: '行政類預付款' },
      { emoji: '🎪', title: '活動設備申請',         desc: '活動設備借用' },
      { emoji: '📋', title: '行政表單申請',         desc: '一般行政申請' },
      { emoji: '📇', title: '名片申請單',           desc: '新印或補印名片' },
    ]
  },
  {
    title: '資產管理', icon: '🏷️',
    items: [
      { emoji: '🏷️', title: '資產外借單',     desc: '公司資產外借登錄' },
      { emoji: '🔁', title: '資產異動申請單', desc: '資產調撥、轉移' },
      { emoji: '🗑️', title: '資產報廢單',     desc: '資產報廢處理' },
    ]
  },
  {
    title: 'MIS相關', icon: '💻',
    items: [
      { emoji: '💻', title: '系統開發/修改/上線申請單', desc: '系統需求提出' },
      { emoji: '🔐', title: 'MIS系統權限申請單',         desc: '系統權限開通或變更' },
    ]
  },
  {
    title: '授信相關', icon: '💳',
    items: [
      { emoji: '⚠️', title: '超額/逾期放行申請表', desc: '額度超出或逾期放行' },
      { emoji: '💳', title: '授信額度申請表',       desc: '客戶授信額度申請' },
    ]
  },
  {
    title: 'NS專區', icon: '🌐',
    items: [
      { emoji: '🌐', title: 'NetSuite 帳號開通 / 異動申請單', desc: 'NetSuite 帳號相關' },
      { emoji: '🌐', title: 'Net Suite Items申請單',          desc: 'NetSuite Item 建檔' },
    ]
  },
  {
    title: '其他', icon: '📋',
    items: [
      { emoji: '🎯', title: 'KOL Radar SaaS 線上購買優惠碼申請', desc: 'KOL Radar 優惠碼' },
      { emoji: '📄', title: '公司文件複本申請單',                desc: '正本文件複本申請' },
      { emoji: '🎫', title: '投標/議價報告書',                   desc: '投標或議價結果報告' },
      { emoji: '📌', title: '標案申請單',                        desc: '標案參與申請' },
      { emoji: '📝', title: '簽呈',                              desc: '通用簽呈' },
    ]
  }
];

const PENDING = [
  {
    status: 'warning', statusText: '審核中', title: '請假申請單',
    who: '王小明（Engineering）', desc: '申請 4/20（一）特休一天，事由：家人返國陪伴。', time: '今天 14:32',
    reviewLog: []  // 第一位審核者
  },
  {
    status: 'warning', statusText: '審核中', title: '費用報銷單',
    who: '李芳華（Marketing）', desc: '客戶餐敘支出 NT$3,200，含 4 位客戶。', time: '今天 11:08',
    reviewLog: [
      { type: 'approve', name: 'Allen Chou', role: '直屬主管', time: '2026/04/15 16:42', comment: '金額合理，發票齊全，同意核准往上呈。' },
    ]
  },
  {
    status: 'warning', statusText: '審核中', title: '加班核准申請單',
    who: '陳大文（Engineering）', desc: '4/18 加班 3 小時，理由：趕上線前測試。', time: '昨天 18:45',
    reviewLog: [
      { type: 'approve', name: 'Steve Lin', role: '直屬主管', time: '2026/04/15 19:20', comment: '確有上線前測試需求，核准。' },
      { type: 'return',  name: '陳育恩',    role: 'HR Manager', time: '2026/04/16 09:05', comment: '請補上實際加班起訖時段，方便計算加班費。' },
    ]
  },
  {
    status: 'warning', statusText: '審核中', title: '出差申請單',
    who: '林美玲（PM Office）', desc: '東京 AI 研討會，出差 3 天（5/5–5/7）。', time: '昨天 10:22',
    reviewLog: [
      { type: 'approve', name: 'Kevin Wang', role: '直屬主管', time: '2026/04/14 14:10', comment: '研討會議程與本季 AI 策略高度相關，同意。' },
      { type: 'approve', name: '陳育恩',     role: 'HR Manager', time: '2026/04/15 10:45', comment: '旅程與預算符合差旅政策。' },
    ]
  },
  {
    // Parallel-sign workflow — all co-signers must approve. The form modal
    // routes to a 退回 / 會簽 footer when signType === 'parallel' (see
    // openFormModal). Status badge stays as the standard 審核中.
    status: 'warning', statusText: '審核中', signType: 'parallel', title: '通用簽呈',
    who: '張庭瑋（PM Office）', desc: '提案：Q3 部門設備預算上修 12%，需各部門主管會簽確認。', time: '今天 10:18',
    reviewLog: [
      { type: 'approve', name: '林雅芳', role: '提案人', time: '2026/05/19 09:30', comment: '附件含市場調查與供應商報價。' },
    ]
  },
];

const REVIEWED = [
  {
    status: 'success', statusText: '已核准', title: '請假申請單',
    who: '王小強（PM Office）', desc: '4/8 特休半天，已自動扣除假餘額。', time: '4/8 16:22',
    reviewLog: [
      { type: 'approve', name: 'Allen Chou', role: '直屬主管', time: '2026/04/08 10:15', comment: '人力可調度，同意。' },
      { type: 'approve', name: '陳育恩',     role: 'HR Manager', time: '2026/04/08 14:30', comment: '' },
      { type: 'approve', name: 'Sega Cheng', role: 'CEO',        time: '2026/04/08 16:22', comment: '' },
    ],
  },
  {
    status: 'success', statusText: '已核准', title: '費用報銷單',
    who: '陳佳慧（Marketing）', desc: '3 月行銷素材採購 NT$12,300。', time: '4/5 11:04',
    reviewLog: [
      { type: 'approve', name: 'Steve Lin',  role: '直屬主管', time: '2026/04/04 18:22', comment: '金額與用途合理，同意。' },
      { type: 'approve', name: '陳育恩',     role: 'HR Manager', time: '2026/04/05 09:50', comment: '' },
      { type: 'approve', name: 'Allen Chou', role: 'Finance',    time: '2026/04/05 11:04', comment: '發票與明細齊全。' },
    ],
  },
  {
    status: 'destructive', statusText: '已退回', title: '出差申請單',
    who: '黃俊傑（Sales）', desc: '缺少完整行程規劃與預算拆解，請補件後重送。', time: '4/3 15:47',
    reviewLog: [
      { type: 'approve', name: 'Kevin Wang', role: '直屬主管', time: '2026/04/02 17:05', comment: '行程目的明確，同意續審。' },
      { type: 'return',  name: '陳育恩',     role: 'HR Manager', time: '2026/04/03 15:47', comment: '缺少行程與預算拆解，請補件後重送。' },
    ],
  },
  {
    status: 'success', statusText: '已核准', title: '加班核准申請單',
    who: '張小明（Engineering）', desc: '3/28 系統上線支援 5 小時。', time: '3/30 09:12',
    reviewLog: [
      { type: 'approve', name: 'Steve Lin', role: '直屬主管', time: '2026/03/29 10:20', comment: '確有上線支援，核准。' },
      { type: 'approve', name: '陳育恩',    role: 'HR Manager', time: '2026/03/30 09:12', comment: '' },
    ],
  },
  {
    status: 'secondary', statusText: '已取消', title: '通用簽呈',
    who: '林美玲（PM Office）', desc: '申請人已自行撤回，原因：提案修改中。', time: '3/28 17:20',
    reviewLog: [
      { type: 'return', name: '林美玲', role: '申請人', time: '2026/03/28 17:20', comment: '提案需修改，先行撤回。' },
    ],
  },
  {
    status: 'success', statusText: '已核准', title: '資產報修申請單',
    who: '李芳華（Marketing）', desc: '辦公室螢幕故障，已指派 IT Helpdesk 處理。', time: '3/27 10:05',
    reviewLog: [
      { type: 'approve', name: 'IT Helpdesk', role: '值班工程師', time: '2026/03/27 10:05', comment: '已指派人員現場處理。' },
    ],
  },
];

// 知會清單 — 你被列為「知會」對象的表單（不需審核，僅資訊通知）。
// Status reflects the form's lifecycle (審核中 / 已核准), not the fact you
// were notified — the 已知會 tab itself communicates that.
const NOTIFIED = [
  {
    status: 'warning', statusText: '審核中', title: '出差申請單',
    who: '李雅琳（行銷部）', desc: '5/12-5/14 赴日參加 MarTech 展覽,已知會行銷部主管。', time: '今天 09:42',
  },
  {
    status: 'success', statusText: '已核准', title: '費用報銷單',
    who: '陳威霖（業務部）', desc: '3 月客戶午餐招待 NT$8,420,已核准並知會財務。', time: '昨天 16:20',
  },
  {
    status: 'success', statusText: '已核准', title: '請假申請單',
    who: '王思怡（產品開發部）', desc: '5/20 全天特休,流程結束已知會直屬主管。', time: '5/10 14:08',
  },
  {
    status: 'success', statusText: '已核准', title: '採購申請單',
    who: '吳勝利（管理部）', desc: 'IT 設備採購 NT$45,000,已核准並知會資訊主管。', time: '5/8 11:30',
  },
];

const SYSTEM_NOTIFS = [
  { status: 'success', statusText: '已核准', title: '你的 4/15 費用報銷已核准', desc: '由 Steve Lin 核准，NT$1,480 將於下月薪資匯入。',             time: '今天 09:14' },
  { status: 'info',    statusText: '系統',   title: '4 月薪資單已發布',          desc: '登入 HR 系統查看明細，若有疑問請於 7 日內提出。',             time: '昨天 17:00' },
  { status: 'warning', statusText: '提醒',   title: '年度健檢預約截止 4/30',     desc: '尚未預約請盡速至 HR 入口選擇時段。',                         time: '4/12' },
  { status: 'info',    statusText: '公告',   title: '差旅新規定於 5/1 生效',     desc: '國際出差預算上限調整，請至政策中心查看。',                   time: '4/10' },
  { status: 'success', statusText: '已核准', title: '你的出差申請 NX-2026-0415 已通過', desc: '由 Allen Chou 核准，請於出差前完成交接單。',       time: '4/9' },
  { status: 'warning', statusText: '提醒',   title: 'VPN 憑證將於 4/25 到期',    desc: '請提前至 IT 入口下載新憑證並更新設定。',                     time: '4/8' },
  { status: 'info',    statusText: '公告',   title: '5/1 勞動節公司放假一天',    desc: '當週工時照常計算，請提前安排工作事項。',                     time: '4/8' },
  { status: 'info',    statusText: '系統',   title: '新版 Nexus OA 平台上線',    desc: '介面全面升級，常用功能與快捷鍵請參閱說明文件。',             time: '4/5' },
  { status: 'success', statusText: '已完成', title: 'Q1 年度績效評核已結案',     desc: '評核報告已寄送至你的 iKala 信箱，可於入口查看摘要。',        time: '4/3' },
  { status: 'warning', statusText: '提醒',   title: '未完成資安合規線上課程',    desc: '截止日 4/30，未完成將影響績效考核。',                         time: '4/1' },
];

const CHAT_HISTORY = [
  { title: '幫我申請下週一請特休',               assist: '請假助理',     time: '今天 14:32', count: 8 },
  { title: '詢問年度特休剩餘天數',               assist: '請假助理',     time: '今天 10:15', count: 4 },
  { title: '4 月份部門支出分析',                 assist: '報表分析師',   time: '昨天 16:48', count: 22 },
  { title: '東京出差交通與飯店建議',             assist: '差旅助理',     time: '昨天 11:03', count: 12 },
  { title: '撰寫客戶提案信件',                   assist: '郵件撰寫',     time: '昨天 09:41', count: 6 },
  { title: '週三產品會議摘要',                   assist: '會議秘書',     time: '4/13 15:22', count: 14 },
  { title: '差旅政策查詢—國外停留上限',         assist: '政策查詢',     time: '4/12 09:48', count: 3 },
  { title: '公司 VPN 連線失敗排除',               assist: 'IT 支援',      time: '4/11 17:05', count: 9 },
  { title: 'Q2 OKR 撰寫草稿',                     assist: 'OKR 教練',     time: '4/10 13:30', count: 18 },
  { title: '報銷發票辨識失敗—麻煩看一下',       assist: '報銷助理',     time: '4/9 10:55',  count: 5 },
  { title: '協助分析最新客戶續約風險',           assist: 'CRM 助理',     time: '4/9 08:45',  count: 11 },
  { title: 'Nexus OA 上線前檢查清單',             assist: '專案管家',     time: '4/8 19:02',  count: 15 },
  { title: '協助草擬員工手冊更新',               assist: '文件助理',     time: '4/8 11:27',  count: 7 },
  { title: '面試評估表撰寫建議',                 assist: '招聘獵頭',     time: '4/7 16:14',  count: 10 },
  { title: '競品定價比較',                       assist: '業務分析',     time: '4/7 09:30',  count: 8 },
  { title: '幫我潤飾週報開頭',                   assist: '文字助理',     time: '4/6 20:10',  count: 3 },
  { title: '資安合規問卷填寫協助',               assist: 'IT 支援',      time: '4/6 11:48',  count: 6 },
  { title: '上個月加班時數統計',                 assist: '考勤分析',     time: '4/5 15:00',  count: 4 },
  { title: '新進員工 onboarding 計畫',            assist: '培訓學長姐',   time: '4/5 10:18',  count: 16 },
  { title: '幫忙擬合約條款草稿',                 assist: '合約法務',     time: '4/4 17:36',  count: 9 },
  { title: '活動行銷文案多版本',                 assist: '品牌行銷',     time: '4/4 14:05',  count: 12 },
  { title: '月度客服 NPS 摘要',                   assist: '客戶關係',     time: '4/3 18:42',  count: 8 },
  { title: '差旅費匯率換算說明',                 assist: '差旅助理',     time: '4/3 09:12',  count: 2 },
  { title: '產品試算表公式出錯排查',             assist: 'IT 支援',      time: '4/2 16:28',  count: 11 },
  { title: '下一季 KPI 設定建議',                 assist: 'OKR 教練',     time: '4/2 10:05',  count: 13 },
  { title: '採購單狀態查詢',                     assist: '採購供應',     time: '4/1 13:54',  count: 5 },
  { title: '簡報版型一致性檢查',                 assist: '設計助理',     time: '3/31 17:21', count: 6 },
  { title: 'AI 產品路線圖討論',                   assist: '專案管家',     time: '3/31 11:40', count: 19 },
  { title: '請假代理人建議名單',                 assist: '請假助理',     time: '3/30 09:08', count: 3 },
  { title: '部門目標拆解',                       assist: 'OKR 教練',     time: '3/30 15:33', count: 10 },
  { title: 'Q1 年度績效彙整',                     assist: '報表分析師',   time: '3/29 14:00', count: 20 },
  { title: 'Sprint retro 記錄整理',                assist: '會議秘書',     time: '3/28 16:55', count: 7 },
  { title: '新版出差規範 FAQ',                    assist: '政策查詢',     time: '3/27 10:42', count: 4 },
  { title: '撰寫對外合作 MOU 草稿',               assist: '合約法務',     time: '3/26 18:20', count: 9 },
];

const FORM_HISTORY = [
  { no: 'NX-2026-0421', title: '請假申請單 — 4/20 特休',         type: '人事考勤', status: 'warning',     statusText: '審核中', date: '2026/4/15 09:24', approver: 'Steve Lin' },
  { no: 'NX-2026-0418', title: '費用報銷 — 客戶餐敘 NT$3,200',   type: '財務費用', status: 'success',     statusText: '已核准', date: '2026/4/14 17:50', approver: 'Allen Chou' },
  { no: 'NX-2026-0415', title: '出差申請 — 東京研討會',           type: '人事考勤', status: 'warning',     statusText: '審核中', date: '2026/4/13 11:08', approver: '—' },
  { no: 'NX-2026-0412', title: '資產報修 — MacBook 螢幕破損',     type: '行政庶務', status: 'success',     statusText: '已核准', date: '2026/4/11 15:32', approver: 'IT Helpdesk' },
  { no: 'NX-2026-0409', title: '物資領用 — 會議白板筆',           type: '行政庶務', status: 'success',     statusText: '已核准', date: '2026/4/9 10:12',  approver: 'Admin Team' },
  { no: 'NX-2026-0406', title: '通用簽呈 — 新工具試用採購',       type: '通用管理', status: 'destructive', statusText: '已退回', date: '2026/4/6 14:47',  approver: 'Finance' },
  { no: 'NX-2026-0402', title: '加班申請 — Q1 結算週末',           type: '人事考勤', status: 'success',     statusText: '已核准', date: '2026/4/2 18:40',  approver: 'Steve Lin' },
  { no: 'NX-2026-0328', title: '差旅費核銷 — 高雄客戶拜訪',       type: '財務費用', status: 'success',     statusText: '已核准', date: '2026/3/28 11:30', approver: 'Allen Chou' },
  { no: 'NX-2026-0330', title: '付款申請 — 雲端服務續約',         type: '財務費用', status: 'warning',     statusText: '審核中', date: '2026/3/30 08:59', approver: 'Finance' },
  { no: 'NX-2026-0325', title: '用印申請 — 合約用印',             type: '行政庶務', status: 'secondary',   statusText: '已取消', date: '2026/3/25 16:22', approver: '—' },
  { no: 'NX-2026-0320', title: '請假申請單 — 3/22 事假',           type: '人事考勤', status: 'success',     statusText: '已核准', date: '2026/3/20 09:05', approver: 'Steve Lin' },
  { no: 'NX-2026-0315', title: '費用報銷 — 團隊聚餐 NT$5,800',    type: '財務費用', status: 'success',     statusText: '已核准', date: '2026/3/15 13:44', approver: 'Allen Chou' },
  { no: 'NX-2026-0312', title: '資產報修 — 會議室投影機',         type: '行政庶務', status: 'success',     statusText: '已核准', date: '2026/3/12 10:02', approver: 'IT Helpdesk' },
  { no: 'NX-2026-0308', title: '職務代理申請 — 3/10–3/12',        type: '人事考勤', status: 'success',     statusText: '已核准', date: '2026/3/8 15:18',  approver: 'Steve Lin' },
  { no: 'NX-2026-0304', title: '預算追加 — 設計軟體授權',         type: '財務費用', status: 'destructive', statusText: '已退回', date: '2026/3/4 09:55',  approver: 'Finance' },
  { no: 'NX-2026-0228', title: '在職證明申請 — 房貸用',           type: '人事考勤', status: 'success',     statusText: '已核准', date: '2026/2/28 14:11', approver: 'HR Team' },
  { no: 'NX-2026-0224', title: '請款單 — 外包設計費',             type: '財務費用', status: 'warning',     statusText: '審核中', date: '2026/2/24 16:30', approver: 'Finance' },
  { no: 'NX-2026-0220', title: '物資領用 — 新進員工設備',         type: '行政庶務', status: 'success',     statusText: '已核准', date: '2026/2/20 11:25', approver: 'Admin Team' },
  { no: 'NX-2026-0215', title: '通用簽呈 — 流程優化提案',         type: '通用管理', status: 'success',     statusText: '已核准', date: '2026/2/15 10:48', approver: 'Steve Lin' },
  { no: 'NX-2026-0210', title: '請假申請單 — 2/14 病假',           type: '人事考勤', status: 'success',     statusText: '已核准', date: '2026/2/10 08:37', approver: 'Steve Lin' },
  { no: 'NX-2026-0205', title: '停車位申請 — B2 月租',             type: '行政庶務', status: 'secondary',   statusText: '已取消', date: '2026/2/5 13:02',  approver: '—' },
  { no: 'NX-2026-0130', title: '個人資料異動 — 通訊地址',         type: '人事考勤', status: 'success',     statusText: '已核准', date: '2026/1/30 17:15', approver: 'HR Team' },
  { no: 'NX-2026-0125', title: '加班申請 — 月底結算',             type: '人事考勤', status: 'success',     statusText: '已核准', date: '2026/1/25 19:22', approver: 'Steve Lin' },
  { no: 'NX-2026-0120', title: '費用報銷 — 1/19 廠商拜訪計程車',  type: '財務費用', status: 'success',     statusText: '已核准', date: '2026/1/20 14:05', approver: 'Allen Chou' },
];

// ---------- Welcome background (random from curated Pexels set) ----------
// Photos from Pexels (free for commercial use).
// URL pattern: https://images.pexels.com/photos/{ID}/pexels-photo-{ID}.jpeg
const WELCOME_BG_IDS = [
  '9002742',   // pexels.com/photo/9002742
  '29355999',  // pexels.com/photo/29355999
  '34939151',  // pexels.com/photo/34939151
  '29586677',  // pexels.com/photo/29586677
  '28494629',  // pexels.com/photo/3d-28494629
  '34939148',  // pexels.com/photo/3d-34939148
  '33839920',  // pexels.com/photo/33839920
  '33797645',  // pexels.com/photo/3d-33797645
  '30004067',  // pexels.com/photo/3d-30004067
  '29764259',  // pexels.com/photo/29764259
  '29612114',  // pexels.com/photo/3d-29612114
  '29474092',  // pexels.com/photo/29474092
  '29450014',  // pexels.com/photo/29450014
  '29172403',  // pexels.com/photo/29172403
];

function applyDailyWelcomeBackground() {
  const hero = document.querySelector('.welcome-hero');
  if (!hero) return;
  // Random pick on each page load
  const id = WELCOME_BG_IDS[Math.floor(Math.random() * WELCOME_BG_IDS.length)];
  const url = `https://images.pexels.com/photos/${id}/pexels-photo-${id}.jpeg?auto=compress&cs=tinysrgb&w=2400&h=1200&fit=crop`;

  // Preload to avoid showing fallback gradient briefly, then swap in
  const img = new Image();
  img.onload = () => {
    hero.style.backgroundImage = `url('${url}')`;
  };
  img.src = url;
}

// ---------- Header: today's date + weather ----------
// Uses Open-Meteo (no API key required, CORS-friendly).
// Default location: Taipei. Weather codes follow the WMO standard.
const HEADER_WEATHER_LOCATION = { name: 'Taipei', lat: 25.0330, lon: 121.5654 };

function weatherIconForCode(code) {
  if (code === 0) return '☀️';
  if (code <= 3) return '🌤️';
  if (code === 45 || code === 48) return '🌫️';
  if (code >= 51 && code <= 67) return '🌧️';
  if (code >= 71 && code <= 77) return '❄️';
  if (code >= 80 && code <= 82) return '🌧️';
  if (code >= 85 && code <= 86) return '❄️';
  if (code >= 95) return '⛈️';
  return '🌤️';
}

function formatHeaderDate(d) {
  const weekdays = ['日', '一', '二', '三', '四', '五', '六'];
  return `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()} 週${weekdays[d.getDay()]}`;
}

async function applyHeaderWeather() {
  const wrap = document.getElementById('header-weather');
  if (!wrap) return;
  const dateEl = wrap.querySelector('.weather-date');
  const iconEl = wrap.querySelector('.weather-icon');
  const tempEl = wrap.querySelector('.weather-temp');
  const cityEl = wrap.querySelector('.weather-city');

  dateEl.textContent = formatHeaderDate(new Date());
  cityEl.textContent = HEADER_WEATHER_LOCATION.name;

  try {
    const { lat, lon } = HEADER_WEATHER_LOCATION;
    const url = `https://api.open-meteo.com/v1/forecast?latitude=${lat}&longitude=${lon}&current=temperature_2m,weather_code&timezone=auto`;
    const res = await fetch(url);
    if (!res.ok) throw new Error('weather fetch failed');
    const data = await res.json();
    const temp = Math.round(data.current?.temperature_2m ?? NaN);
    const code = data.current?.weather_code ?? 0;
    if (!Number.isFinite(temp)) throw new Error('invalid temp');
    iconEl.textContent = weatherIconForCode(code);
    tempEl.textContent = `${temp}°`;
  } catch {
    // Hide weather pieces if fetch fails, keep the date
    wrap.querySelector('.weather-sep').style.display  = 'none';
    iconEl.style.display = 'none';
    tempEl.style.display = 'none';
    cityEl.style.display = 'none';
  }
}

// ---------- Utilities ----------
const $  = (sel, ctx = document) => ctx.querySelector(sel);
const $$ = (sel, ctx = document) => [...ctx.querySelectorAll(sel)];

function el(tag, cls, html) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (html !== undefined) e.innerHTML = html;
  return e;
}

function iconsRefresh() {
  if (window.lucide) lucide.createIcons();
}

/**
 * Close any other `.dropdown.open` (keeping `dd` untouched), then toggle
 * `dd.open`. Call this from dropdown trigger clicks so only one dropdown
 * can be visible at a time — clicking another trigger while A is open
 * cleanly switches to B.
 */
function toggleDropdownExclusive(dd) {
  if (!dd) return;
  document.querySelectorAll('.dropdown.open').forEach(other => {
    if (other !== dd) other.classList.remove('open');
  });
  dd.classList.toggle('open');
}

/**
 * Show a toast notification at the bottom-right of the viewport. Auto-dismisses
 * after `duration` ms (default 3000). `variant`: 'success' | 'error' | 'info'.
 *
 * Optional `action`: { label, onClick } — adds an inline button (e.g. 還原).
 * Clicking it dismisses the toast and fires onClick. When an action is
 * present, `duration` defaults to 6000ms to give time to react.
 */
function showToast({ title, desc = '', variant = 'success', duration, icon, action } = {}) {
  const layer = document.getElementById('toast-layer');
  if (!layer) return;
  const iconName = icon || (variant === 'error' ? 'alert-circle' : variant === 'info' ? 'info' : 'check-circle');
  const resolvedDuration = duration ?? (action ? 6000 : 3000);
  const toast = el('div', `toast toast-${variant}${action ? ' toast-with-action' : ''}`);
  toast.innerHTML = `
    <div class="toast-icon"><i data-lucide="${iconName}" class="icon"></i></div>
    <div class="toast-content">
      <div class="toast-title">${title || ''}</div>
      ${desc ? `<div class="toast-desc">${desc}</div>` : ''}
    </div>
    ${action ? `<button class="toast-action-btn" type="button">${action.label}</button>` : ''}
  `;
  layer.appendChild(toast);
  iconsRefresh();
  requestAnimationFrame(() => toast.classList.add('toast-open'));

  const dismiss = () => {
    toast.classList.remove('toast-open');
    setTimeout(() => toast.remove(), 240);
  };
  if (action) {
    toast.querySelector('.toast-action-btn')?.addEventListener('click', () => {
      dismiss();
      action.onClick?.();
    });
  }
  setTimeout(dismiss, resolvedDuration);
}

/**
 * Play the ✓ success overlay inside the form modal. Resolves after the full
 * animation (≈780ms) so callers can chain a close / advance.
 */
function playFormModalSuccess({ title = '已送出', desc = '' } = {}) {
  const overlay = document.getElementById('form-modal-success');
  if (!overlay) return Promise.resolve();
  const titleEl = document.getElementById('form-modal-success-title');
  const descEl = document.getElementById('form-modal-success-desc');
  if (titleEl) titleEl.textContent = title;
  if (descEl)  descEl.textContent = desc;
  // Restart CSS animations by cloning the overlay's inner content — toggling
  // `hidden` alone doesn't reset keyframes on subsequent plays.
  const clone = overlay.cloneNode(true);
  overlay.parentNode.replaceChild(clone, overlay);
  clone.hidden = false;
  return new Promise(resolve => {
    setTimeout(() => {
      clone.hidden = true;
      resolve();
    }, 780);
  });
}

/**
 * Render pagination controls into a host element. Hides itself when there's
 * only one page. Used by the pending/reviewed/drafts/applied lists.
 */
function renderPagination({ hostId, totalItems, page, pageSize, onPageChange }) {
  const host = document.getElementById(hostId);
  if (!host) return;
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  if (totalPages <= 1) { host.innerHTML = ''; return; }
  host.innerHTML = `
    <div class="p-small muted">共 ${totalItems} 筆</div>
    <div class="pagination-pages">
      <button class="page-btn" data-dir="prev" ${page === 1 ? 'disabled' : ''}>
        <i data-lucide="chevron-left" class="icon" style="width:14px;height:14px"></i>
      </button>
      ${Array.from({ length: totalPages }, (_, i) =>
        `<button class="page-btn ${i + 1 === page ? 'active' : ''}" data-page="${i + 1}">${i + 1}</button>`
      ).join('')}
      <button class="page-btn" data-dir="next" ${page === totalPages ? 'disabled' : ''}>
        <i data-lucide="chevron-right" class="icon" style="width:14px;height:14px"></i>
      </button>
    </div>
  `;
  host.querySelectorAll('.page-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      let next = page;
      if (btn.dataset.dir === 'prev' && page > 1) next = page - 1;
      else if (btn.dataset.dir === 'next' && page < totalPages) next = page + 1;
      else if (btn.dataset.page) next = parseInt(btn.dataset.page, 10);
      if (next !== page) onPageChange(next);
    });
  });
  iconsRefresh();
}

const LIST_PAGE_SIZE = 10;

/**
 * Short display format (no year unless explicitly given):
 *   Keeps:  今天 HH:mm / 昨天 HH:mm
 *   Full:   YYYY/MM/DD[ HH:mm]  (zero-padded)
 *   Short:  MM/DD[ HH:mm]       (zero-padded)
 */
function fmtDate(str) {
  if (!str) return str;
  const s = String(str).trim();
  if (s.startsWith('今天') || s.startsWith('昨天')) return s;
  const full = s.match(/^(\d{4})[\/\-](\d{1,2})[\/\-](\d{1,2})(\s+\d{1,2}:\d{2})?$/);
  if (full) {
    return `${full[1]}/${String(full[2]).padStart(2,'0')}/${String(full[3]).padStart(2,'0')}${full[4] || ''}`;
  }
  const short = s.match(/^(\d{1,2})\/(\d{1,2})(\s+\d{1,2}:\d{2})?$/);
  if (short) {
    return `${String(short[1]).padStart(2,'0')}/${String(short[2]).padStart(2,'0')}${short[3] || ''}`;
  }
  return s;
}

/**
 * Long display format — always include year (except for 今天/昨天):
 *   今天 HH:mm / 昨天 HH:mm → kept as-is (zero-padded via fmtDate)
 *   YYYY/M/D[ HH:mm] → YYYY/MM/DD[ HH:mm]
 *   M/D[ HH:mm]      → (current year)/MM/DD[ HH:mm]
 */
function fmtDateLong(str) {
  if (!str) return str;
  const s = String(str).trim();
  if (s.startsWith('今天') || s.startsWith('昨天')) return fmtDate(s);
  const year = new Date().getFullYear();
  const full = s.match(/^(\d{4})[\/\-](\d{1,2})[\/\-](\d{1,2})(\s+\d{1,2}:\d{2})?$/);
  if (full) {
    return `${full[1]}/${String(full[2]).padStart(2,'0')}/${String(full[3]).padStart(2,'0')}${full[4] || ''}`;
  }
  const short = s.match(/^(\d{1,2})\/(\d{1,2})(\s+\d{1,2}:\d{2})?$/);
  if (short) {
    return `${year}/${String(short[1]).padStart(2,'0')}/${String(short[2]).padStart(2,'0')}${short[3] || ''}`;
  }
  return s;
}

// ---------- Alert Dialog (reusable confirm) ----------
let _alertDialogCb = null;
function confirmDialog({ title = '確定執行此操作？', desc = '此動作無法復原。', confirmText = '刪除', onConfirm } = {}) {
  const modal = $('#alert-dialog');
  if (!modal) return;
  $('#alert-dialog-title').textContent = title;
  $('#alert-dialog-desc').textContent = desc;
  $('#alert-dialog-confirm-label').textContent = confirmText;
  _alertDialogCb = onConfirm || null;
  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  iconsRefresh();
}
function closeAlertDialog() {
  const modal = $('#alert-dialog');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
  _alertDialogCb = null;
}
function initAlertDialog() {
  const modal = $('#alert-dialog');
  if (!modal) return;
  $('#alert-dialog-cancel')?.addEventListener('click', closeAlertDialog);
  $('#alert-dialog-confirm')?.addEventListener('click', () => {
    const cb = _alertDialogCb;
    closeAlertDialog();
    if (typeof cb === 'function') cb();
  });
  modal.addEventListener('click', (e) => { if (e.target === modal) closeAlertDialog(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && modal.classList.contains('open')) closeAlertDialog();
  });
}

// ---------- Header AI toggle ----------
// Maps view IDs to their right-side AI chat panel selector
const AI_PANEL_MAP = {
  'view-notifications': '.ai-review-pane',
  'view-forms': '.form-right',
  'view-dashboard': '.dash-right',
  'view-tasks': '.task-right-ai',
  'view-assistants': '.assistants-ai-pane',
};

function initHeaderAIBtn() {
  const btn = $('#header-ai-btn');
  if (!btn) return;

  // Default: all AI panels closed on initial load
  Object.values(AI_PANEL_MAP).forEach(selector => {
    $$(selector).forEach(p => p.classList.add('ai-panel-hidden'));
  });
  btn.classList.remove('ai-btn-active');
  // Initial view is home — show icon-only pill and make it visible
  const initialView = $('.view.active');
  if (initialView?.id === 'view-home') {
    btn.classList.add('ai-btn-icon-only');
    btn.style.display = '';
  }

  btn.addEventListener('click', () => {
    toggleAIPanel(btn);
  });

  // Close buttons inside AI panels
  $$('.ai-panel-close').forEach(closeBtn => {
    closeBtn.addEventListener('click', () => {
      toggleAIPanel(btn);
    });
  });
}

function toggleAIPanel(btn) {
  const activeView = $('.view.active');
  if (!activeView) return;

  const panelSelector = AI_PANEL_MAP[activeView.id];
  if (panelSelector) {
    const panel = activeView.querySelector(panelSelector);
    if (panel) {
      panel.classList.toggle('ai-panel-hidden');
      btn.classList.toggle('ai-btn-active', !panel.classList.contains('ai-panel-hidden'));
    }
  } else {
    // Home (and any other view without a panel) — open AI chat inner page
    switchView('chat', { assistant: ASSISTANTS[0] });
  }
}

// Force-close every AI panel across all views (used when opening the mobile
// sidebar drawer so the fullscreen AI pane doesn't overlap).
function closeAllAIPanels() {
  Object.values(AI_PANEL_MAP).forEach(selector => {
    $$(selector).forEach(p => p.classList.add('ai-panel-hidden'));
  });
  $('#header-ai-btn')?.classList.remove('ai-btn-active');
}

// ---------- Home header scroll fade ----------
function initHomeHeaderScroll() {
  const viewHome = $('#view-home');
  const header = $('.header');
  if (!viewHome || !header) return;

  viewHome.addEventListener('scroll', () => {
    const scrollY = viewHome.scrollTop;
    const opacity = Math.min(scrollY / 100, 1);
    header.style.setProperty('--header-bg-opacity', opacity);
    header.classList.toggle('header-scrolled', opacity >= 1);
  });
}

// ---------- Navigation ----------
const VIEW_TITLES = {
  home: '主頁',
  tasks: '工作任務',
  notifications: '待辦審核',
  forms: '表單申請',
  dashboard: '洞察報表',
  assistants: 'AI 助理',
  chat: 'AI 助理對話',
  history: '歷史紀錄',
  workspace: '工作區設定'
};

function switchView(view, opts = {}) {
  $$('.view').forEach(v => v.classList.remove('active'));
  const target = $('#view-' + view);
  if (target) target.classList.add('active');

  // Sidebar active state — when on chat view, highlight assistants in sidebar.
  $$('.sidebar-item').forEach(i => i.classList.remove('active'));
  const highlightView = view === 'chat' ? 'assistants' : view;
  const highlight = $(`.sidebar-item[data-view="${highlightView}"]`);
  if (highlight) highlight.classList.add('active');

  // Close user dropdown when switching via avatar menu
  const userDD = $('#user-dropdown');
  if (userDD) userDD.classList.remove('open');

  // Toggle transparent header on home view
  const main = $('.main');
  if (main) main.classList.toggle('home-active', view === 'home');

  // Collapse left sidebar when entering the AI assistant chat view
  const app = $('.app');
  if (app) app.classList.toggle('sidebar-collapsed', view === 'chat');
  // Workspace replaces the main sidebar with its own admin nav
  if (app) app.classList.toggle('workspace-mode', view === 'workspace');

  // Toggle header: weather vs chat title vs sidebar toggle only
  const weather = $('#header-weather');
  const chatTitle = $('#header-chat-title');
  const sidebarToggle = $('#sidebar-toggle-btn');
  const chatHistoryToggle = $('#chat-history-toggle-btn');
  if (weather && chatTitle) {
    if (view === 'chat') {
      weather.style.display = 'none';
      chatTitle.style.display = '';
      if (sidebarToggle) sidebarToggle.style.display = 'none';
      if (chatHistoryToggle) chatHistoryToggle.style.display = '';
      const activeChat = $('.chat-history-item.active div');
      chatTitle.textContent = activeChat ? activeChat.textContent : '';
    } else if (view === 'home') {
      weather.style.display = '';
      chatTitle.style.display = 'none';
      if (sidebarToggle) sidebarToggle.style.display = '';
      if (chatHistoryToggle) chatHistoryToggle.style.display = 'none';
    } else {
      weather.style.display = 'none';
      chatTitle.style.display = 'none';
      if (sidebarToggle) sidebarToggle.style.display = '';
      if (chatHistoryToggle) chatHistoryToggle.style.display = 'none';
    }
  }
  // Always start the chat-history drawer closed when (re)entering the view
  $('.chat-layout')?.classList.remove('chat-history-open');

  // On views with their own internal sidebar (chat, workspace), move the
  // header INTO that view's main pane so it spans only the right column.
  // The internal sidebar has no header above it.
  const header = $('.header');
  const subPane = view === 'chat' ? $('.chat-main')
                : view === 'workspace' ? $('.workspace-main')
                : null;
  if (header && main) {
    if (subPane) {
      if (header.parentElement !== subPane) {
        subPane.insertBefore(header, subPane.firstChild);
      }
    } else {
      if (header.parentElement !== main) {
        main.insertBefore(header, main.firstChild);
      }
    }
  }

  // Reset AI panel state for new view — default closed, user must click to open
  const aiBtn = $('#header-ai-btn');
  const activeViewEl = $('.view.active');
  if (activeViewEl) {
    const panelSelector = AI_PANEL_MAP[activeViewEl.id];
    if (panelSelector) {
      const panel = activeViewEl.querySelector(panelSelector);
      if (panel) panel.classList.add('ai-panel-hidden');
    }
  }
  if (aiBtn) {
    const hideBtn = view === 'chat' || view === 'history' || view === 'workspace';
    aiBtn.style.display = hideBtn ? 'none' : '';
    aiBtn.classList.remove('ai-btn-active');
    // On home, show button in icon-only pill form (gradient preserved)
    aiBtn.classList.toggle('ai-btn-icon-only', view === 'home');
  }

  // Scroll content to top
  const content = $('.content');
  if (content) content.scrollTop = 0;

  // Auto-focus the primary textarea of the view so the user can type
  // immediately (home welcome / assistant chat).
  const AUTOFOCUS_BY_VIEW = {
    home: '#welcome-input-textarea',
    chat: '#chat-view-textarea'
  };
  const autofocusSel = AUTOFOCUS_BY_VIEW[view];
  if (autofocusSel) {
    requestAnimationFrame(() => {
      const t = $(autofocusSel);
      if (t) t.focus({ preventScroll: true });
    });
  }
}

// ---------- Custom Select Component ----------
function initCustomSelect(container) {
  const options = (container.dataset.options || '').split(',').filter(Boolean);
  const placeholder = container.dataset.placeholder || '選擇';

  container.innerHTML = `
    <div class="custom-select-trigger" tabindex="0">
      <span class="custom-select-text placeholder">${placeholder}</span>
      <i data-lucide="chevron-down" class="icon chevron"></i>
    </div>
    <div class="custom-select-menu">
      ${options.map(o => `<div class="custom-select-option" data-value="${o}">${o}</div>`).join('')}
    </div>
  `;
  iconsRefresh();

  const trigger = container.querySelector('.custom-select-trigger');
  const textEl = container.querySelector('.custom-select-text');
  container._value = '';

  trigger.addEventListener('click', (e) => {
    e.stopPropagation();
    $$('.custom-select.open, .custom-datepicker.open').forEach(el => { if (el !== container) el.classList.remove('open'); });
    container.classList.toggle('open');
    if (container.classList.contains('open')) {
      const menu = container.querySelector('.custom-select-menu');
      const rect = trigger.getBoundingClientRect();
      menu.style.top = rect.bottom + 4 + 'px';
      menu.style.left = rect.left + 'px';
      menu.style.width = rect.width + 'px';
    }
  });

  container.querySelectorAll('.custom-select-option').forEach(opt => {
    opt.addEventListener('click', (e) => {
      e.stopPropagation();
      container._value = opt.dataset.value;
      textEl.textContent = opt.dataset.value;
      textEl.classList.remove('placeholder');
      container.querySelectorAll('.custom-select-option').forEach(o => o.classList.remove('selected'));
      opt.classList.add('selected');
      container.classList.remove('open');
    });
  });

  // Public API
  Object.defineProperty(container, 'value', {
    get() { return container._value; },
    set(v) {
      container._value = v;
      if (v) {
        textEl.textContent = v;
        textEl.classList.remove('placeholder');
        container.querySelectorAll('.custom-select-option').forEach(o => {
          o.classList.toggle('selected', o.dataset.value === v);
        });
      } else {
        textEl.textContent = placeholder;
        textEl.classList.add('placeholder');
        container.querySelectorAll('.custom-select-option').forEach(o => o.classList.remove('selected'));
      }
    }
  });
}

// ---------- Custom Datepicker Component ----------
function initCustomDatepicker(container) {
  const weekdays = ['日','一','二','三','四','五','六'];
  let currentDate = new Date();
  let selectedDate = null;
  let viewYear = currentDate.getFullYear();
  let viewMonth = currentDate.getMonth();

  container.innerHTML = `
    <div class="custom-datepicker-trigger" tabindex="0">
      <i data-lucide="calendar" class="icon"></i>
      <span class="dp-text placeholder">選擇日期</span>
    </div>
    <div class="dp-popover">
      <div class="dp-header">
        <button class="icon-button dp-prev" type="button"><i data-lucide="chevron-left" class="icon"></i></button>
        <span class="p-medium dp-title"></span>
        <button class="icon-button dp-next" type="button"><i data-lucide="chevron-right" class="icon"></i></button>
      </div>
      <div class="dp-weekdays">${weekdays.map(d => `<div class="dp-weekday">${d}</div>`).join('')}</div>
      <div class="dp-days"></div>
    </div>
  `;
  iconsRefresh();

  const trigger = container.querySelector('.custom-datepicker-trigger');
  const textEl = container.querySelector('.dp-text');
  const titleEl = container.querySelector('.dp-title');
  const daysEl = container.querySelector('.dp-days');

  function renderCalendar() {
    titleEl.textContent = `${viewYear} 年 ${viewMonth + 1} 月`;
    const firstDay = new Date(viewYear, viewMonth, 1).getDay();
    const daysInMonth = new Date(viewYear, viewMonth + 1, 0).getDate();
    const daysInPrevMonth = new Date(viewYear, viewMonth, 0).getDate();
    const today = new Date();

    let html = '';
    // Previous month
    for (let i = firstDay - 1; i >= 0; i--) {
      html += `<button class="dp-day muted" type="button" data-date="">${daysInPrevMonth - i}</button>`;
    }
    // Current month
    for (let d = 1; d <= daysInMonth; d++) {
      const dateStr = `${viewYear}-${String(viewMonth + 1).padStart(2, '0')}-${String(d).padStart(2, '0')}`;
      const isToday = (d === today.getDate() && viewMonth === today.getMonth() && viewYear === today.getFullYear());
      const isSelected = selectedDate === dateStr;
      html += `<button class="dp-day${isToday ? ' today' : ''}${isSelected ? ' selected' : ''}" type="button" data-date="${dateStr}">${d}</button>`;
    }
    // Next month
    const totalCells = firstDay + daysInMonth;
    const remaining = (7 - totalCells % 7) % 7;
    for (let i = 1; i <= remaining; i++) {
      html += `<button class="dp-day muted" type="button" data-date="">${i}</button>`;
    }
    daysEl.innerHTML = html;

    daysEl.querySelectorAll('.dp-day[data-date]:not(.muted)').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        selectedDate = btn.dataset.date;
        container._value = selectedDate;
        textEl.textContent = selectedDate.replace(/-/g, '/');
        textEl.classList.remove('placeholder');
        container.classList.remove('open');
        renderCalendar();
      });
    });
  }

  trigger.addEventListener('click', (e) => {
    e.stopPropagation();
    $$('.custom-select.open, .custom-datepicker.open').forEach(el => { if (el !== container) el.classList.remove('open'); });
    container.classList.toggle('open');
    if (container.classList.contains('open')) {
      renderCalendar();
      const popover = container.querySelector('.dp-popover');
      const rect = trigger.getBoundingClientRect();
      popover.style.top = rect.bottom + 4 + 'px';
      popover.style.left = rect.left + 'px';
    }
  });

  container.querySelector('.dp-prev').addEventListener('click', (e) => {
    e.stopPropagation();
    viewMonth--;
    if (viewMonth < 0) { viewMonth = 11; viewYear--; }
    renderCalendar();
  });
  container.querySelector('.dp-next').addEventListener('click', (e) => {
    e.stopPropagation();
    viewMonth++;
    if (viewMonth > 11) { viewMonth = 0; viewYear++; }
    renderCalendar();
  });

  container._value = '';
  Object.defineProperty(container, 'value', {
    get() { return container._value; },
    set(v) {
      container._value = v;
      if (v) {
        selectedDate = v;
        textEl.textContent = v.replace(/-/g, '/');
        textEl.classList.remove('placeholder');
        const parts = v.split('-');
        viewYear = parseInt(parts[0]);
        viewMonth = parseInt(parts[1]) - 1;
      } else {
        selectedDate = null;
        textEl.textContent = '選擇日期';
        textEl.classList.add('placeholder');
      }
    }
  });

  renderCalendar();
}

// Close custom dropdowns on outside click
document.addEventListener('click', () => {
  $$('.custom-select.open, .custom-datepicker.open').forEach(el => el.classList.remove('open'));
});

// ---------- Tasks data & rendering ----------
const TASKS = [
  { name: 'Sync with Lily/Adam', date: '2026-04-16', member: 'Tammy Chen', product: 'AI Product', category: 'Meeting', hours: 1.5 },
  { name: 'Nexus OA UI', date: '2026-04-16', member: 'Tammy Chen', product: 'Nexus', category: 'UI/UX Design', hours: 4.5 },
  { name: 'Nexus discuss with Sarah', date: '2026-04-16', member: 'Tammy Chen', product: 'Nexus', category: 'Discuss', hours: 1 },
  { name: '用戶訪談：醫債生態', date: '2026-04-15', member: 'Viya Hsieh', product: 'Koir', category: 'UI/UX Testing', hours: 1.5 },
  { name: 'API 文件整理', date: '2026-04-15', member: 'Tammy Chen', product: 'Nexus', category: 'Development', hours: 2 },
  { name: '週會簡報準備', date: '2026-04-15', member: 'Tammy Chen', product: 'AI Product', category: 'Other', hours: 1 },
  { name: '競品分析報告', date: '2026-04-14', member: 'Tammy Chen', product: 'AI Product', category: 'Research', hours: 3 },
  { name: 'Design review', date: '2026-04-14', member: 'Tammy Chen', product: 'Nexus', category: 'Meeting', hours: 1.5 },
  // Ella Wang
  { name: 'Koir Landing Page 視覺', date: '2026-04-16', member: 'Ella Wang', product: 'Koir', category: 'UI/UX Design', hours: 5 },
  { name: '設計系統 token 盤點', date: '2026-04-15', member: 'Ella Wang', product: 'Nexus', category: 'UI/UX Design', hours: 3.5 },
  { name: '週會', date: '2026-04-15', member: 'Ella Wang', product: 'AI Product', category: 'Meeting', hours: 1 },
  { name: '新版 Icon 設計', date: '2026-04-14', member: 'Ella Wang', product: 'Nexus', category: 'UI/UX Design', hours: 4 },
  { name: '競品視覺調研', date: '2026-04-11', member: 'Ella Wang', product: 'Koir', category: 'Research', hours: 3 },
  { name: 'Sprint Kick-off', date: '2026-04-08', member: 'Ella Wang', product: 'AI Product', category: 'Meeting', hours: 1.5 },
  // Viya Hsieh
  { name: 'Koir 原型測試分析', date: '2026-04-16', member: 'Viya Hsieh', product: 'Koir', category: 'UI/UX Testing', hours: 3 },
  { name: '訪談腳本撰寫', date: '2026-04-14', member: 'Viya Hsieh', product: 'Koir', category: 'Research', hours: 2.5 },
  { name: '內部 Sync', date: '2026-04-14', member: 'Viya Hsieh', product: 'AI Product', category: 'Meeting', hours: 1 },
  { name: '醫師訪談 x3', date: '2026-04-11', member: 'Viya Hsieh', product: 'Koir', category: 'UI/UX Testing', hours: 4.5 },
  { name: '使用者旅程整理', date: '2026-04-09', member: 'Viya Hsieh', product: 'Nexus', category: 'Research', hours: 3 },
  // Kevin Lin
  { name: 'Nexus API 串接', date: '2026-04-16', member: 'Kevin Lin', product: 'Nexus', category: 'Development', hours: 5 },
  { name: 'Bug fix: 表單送出錯誤', date: '2026-04-15', member: 'Kevin Lin', product: 'Nexus', category: 'Development', hours: 2 },
  { name: 'Code review', date: '2026-04-15', member: 'Kevin Lin', product: 'AI Product', category: 'Discuss', hours: 1 },
  { name: '架構 refactor 討論', date: '2026-04-11', member: 'Kevin Lin', product: 'Nexus', category: 'Discuss', hours: 1.5 },
  { name: 'DB schema 設計', date: '2026-04-09', member: 'Kevin Lin', product: 'Nexus', category: 'Development', hours: 4 },
  // Allen Wu
  { name: 'AI 助理 prompt 調整', date: '2026-04-16', member: 'Allen Wu', product: 'AI Product', category: 'Development', hours: 4 },
  { name: 'Eval 指標盤點', date: '2026-04-15', member: 'Allen Wu', product: 'AI Product', category: 'Research', hours: 2.5 },
  { name: 'Stand-up', date: '2026-04-15', member: 'Allen Wu', product: 'AI Product', category: 'Meeting', hours: 0.5 },
  { name: '模型比較報告', date: '2026-04-11', member: 'Allen Wu', product: 'AI Product', category: 'Research', hours: 3.5 },
  { name: 'Knowledge base 整理', date: '2026-04-08', member: 'Allen Wu', product: 'Nexus', category: 'Other', hours: 2 },
];

const DEPT_LEAVES = [
  { member: 'Tammy Chen', days: 1,   type: '特休' },
  { member: 'Ella Wang',  days: 2,   type: '特休' },
  { member: 'Viya Hsieh', days: 0.5, type: '事假' },
  { member: 'Kevin Lin',  days: 0,   type: '—' },
  { member: 'Allen Wu',   days: 1,   type: '病假' },
  { member: 'Sarah Chang',days: 1.5, type: '特休' },
  { member: 'Jason Lee',  days: 0,   type: '—' },
  { member: 'Grace Lin',  days: 2,   type: '病假' },
  { member: 'David Hsu',  days: 0.5, type: '事假' },
  { member: 'Nina Yang',  days: 1,   type: '特休' },
  { member: 'Brian Kuo',  days: 0,   type: '—' },
  { member: 'Hannah Liu', days: 1,   type: '特休' },
  { member: 'Leo Wu',     days: 3,   type: '特休' },
  { member: 'Cindy Tsai', days: 0.5, type: '病假' },
];

const TODOS = [
  { text: '完成 Nexus OA 表單流程設計', done: true, date: '04/16' },
  { text: '準備週五 sprint demo', done: false, date: '04/18' },
  { text: '回覆 Sarah 的設計反饋', done: false, date: '04/17' },
  { text: '更新 API 文件', done: false, date: '04/17' },
  { text: '整理用戶訪談筆記', done: true, date: '04/15' },
];

let taskFilterMembers = new Set(['Tammy Chen']);
const _taskToday = new Date();
let taskViewYear = _taskToday.getFullYear();
let taskViewMonth = _taskToday.getMonth(); // 0-indexed

const TEAM_MEMBERS = [
  { name: 'Tammy Chen', initial: 'T' },
  { name: 'Ella Wang', initial: 'E' },
  { name: 'Viya Hsieh', initial: 'V' },
  { name: 'Kevin Lin', initial: 'K' },
  { name: 'Allen Wu', initial: 'A' },
  { name: 'Sarah Chang', initial: 'S' },
  { name: 'Jason Lee', initial: 'J' },
  { name: 'Grace Lin', initial: 'G' },
  { name: 'David Hsu', initial: 'D' },
  { name: 'Nina Yang', initial: 'N' },
  { name: 'Brian Kuo', initial: 'B' },
  { name: 'Hannah Liu', initial: 'H' },
  { name: 'Leo Wu', initial: 'L' },
  { name: 'Cindy Tsai', initial: 'C' },
];

function updateTaskMonthLabel() {
  const title = $('#task-month-title');
  if (title) title.textContent = `${taskViewYear} 年 ${taskViewMonth + 1} 月 工作紀錄`;
}

function shiftTaskMonth(delta) {
  const d = new Date(taskViewYear, taskViewMonth + delta, 1);
  taskViewYear = d.getFullYear();
  taskViewMonth = d.getMonth();
  updateTaskMonthLabel();
  renderTasksByDate();
}

function initTaskMonthNav() {
  const prev = $('#task-month-prev');
  const next = $('#task-month-next');
  if (prev) prev.addEventListener('click', () => shiftTaskMonth(-1));
  if (next) next.addEventListener('click', () => shiftTaskMonth(1));
  updateTaskMonthLabel();
}

function renderTasksByDate() {
  const container = $('#task-date-list');
  if (!container) return;
  container.innerHTML = '';

  const filtered = TASKS.filter(t => taskFilterMembers.has(t.member));

  // Group by date, keep only tasks inside the selected month
  const groups = {};
  filtered.forEach(t => {
    const [y, m] = t.date.split('-').map(Number);
    if (y !== taskViewYear || m !== taskViewMonth + 1) return;
    if (!groups[t.date]) groups[t.date] = [];
    groups[t.date].push(t);
  });

  const dates = new Set(Object.keys(groups));

  // Scaffold: when viewing the current month, always include recent days
  const today = new Date();
  const isCurrentMonth = today.getFullYear() === taskViewYear && today.getMonth() === taskViewMonth;
  if (isCurrentMonth) {
    for (let i = 0; i < 7; i++) {
      const d = new Date(today);
      d.setDate(d.getDate() - i);
      if (d.getFullYear() === taskViewYear && d.getMonth() === taskViewMonth) {
        const key = `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,'0')}-${String(d.getDate()).padStart(2,'0')}`;
        dates.add(key);
      }
    }
  }

  const sortedDates = [...dates].sort((a, b) => b.localeCompare(a));

  if (sortedDates.length === 0) {
    const empty = el('div', 'task-month-empty');
    empty.innerHTML = `<span class="p-small muted">此月份尚無任務紀錄</span>`;
    container.appendChild(empty);
    return;
  }

  sortedDates.forEach(date => {
    const d = new Date(date + 'T00:00:00');
    const label = `${String(d.getMonth()+1).padStart(2,'0')}/${String(d.getDate()).padStart(2,'0')}`;
    const weekday = ['日','一','二','三','四','五','六'][d.getDay()];
    const tasks = groups[date] || [];
    const totalHours = tasks.reduce((s, t) => s + t.hours, 0);
    const isEmpty = tasks.length === 0;

    const TARGET = 7;
    const pct = Math.min(totalHours / TARGET * 100, 100);
    const isFull = totalHours >= TARGET;
    const isOver = totalHours > TARGET;

    const group = el('div', 'task-date-group');
    group.innerHTML = `
      <div class="task-date-head">
        <div class="task-date-label">
          <span class="h4">${label}（${weekday}）</span>
        </div>
        <div class="task-progress">
          <div class="task-progress-bar">
            <div class="task-progress-fill ${isFull ? 'full' : ''} ${isOver ? 'over' : ''}" style="transform:scaleX(${(pct/100).toFixed(4)})"></div>
          </div>
          <span class="task-progress-text ${isOver ? 'over' : isFull ? 'full' : ''}">${totalHours}/${TARGET}h</span>
        </div>
      </div>
      <div class="task-card-list">
        ${isEmpty ? `
          <div class="task-empty">
            <span class="p-small muted">今日無任務</span>
            <button class="task-add-btn"><i data-lucide="plus" class="icon"></i> 新增工作項目</button>
          </div>
        ` : tasks.map(t => {
          const idx = TASKS.indexOf(t);
          return `
          <div class="task-card ${isOver ? 'task-card--over' : isFull ? 'task-card--full' : ''}" data-task-idx="${idx}" draggable="true">
            <div class="task-card-main">
              <span class="task-card-name">${t.name}</span>
              <div class="task-card-tags">
                <span class="badge badge-secondary">${t.product}</span>
                <span class="badge badge-secondary">${t.category}</span>
              </div>
            </div>
            <div class="task-card-right">
              <div class="task-card-hours">${t.hours}h</div>
              <div class="task-card-actions">
                <button class="task-action-btn" aria-label="編輯" data-action="edit">
                  <i data-lucide="pencil" class="icon"></i>
                </button>
                <button class="task-action-btn task-action-delete" aria-label="刪除" data-action="delete">
                  <i data-lucide="trash-2" class="icon"></i>
                </button>
              </div>
            </div>
          </div>`;
        }).join('')}
      </div>
    `;
    container.appendChild(group);

    const addBtn = group.querySelector('.task-add-btn');
    if (addBtn) addBtn.addEventListener('click', () => {
      openTaskModal();
      $('#task-f-date').value = date;
    });
    group.querySelectorAll('.task-card[data-task-idx]').forEach(card => {
      card.addEventListener('click', (e) => {
        const actionBtn = e.target.closest('[data-action]');
        const idx = parseInt(card.dataset.taskIdx);
        if (actionBtn?.dataset.action === 'delete') {
          e.stopPropagation();
          confirmDialog({
            title: '確定要刪除此任務？',
            desc: `「${TASKS[idx].name}」將被移除，此動作無法復原。`,
            confirmText: '刪除',
            onConfirm: () => {
              TASKS.splice(idx, 1);
              renderTasksByDate();
            },
          });
          return;
        }
        openTaskModal(idx);
      });
      // Drag & drop — move the task to another date by dragging its card
      card.addEventListener('dragstart', (e) => {
        const idx = card.dataset.taskIdx;
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', idx);
        card.classList.add('task-card-dragging');
      });
      card.addEventListener('dragend', () => {
        card.classList.remove('task-card-dragging');
        document.querySelectorAll('.task-date-group-drop-target')
          .forEach(el => el.classList.remove('task-date-group-drop-target'));
      });
    });
    // Accept task cards dropped onto this date group
    group.addEventListener('dragover', (e) => {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
      group.classList.add('task-date-group-drop-target');
    });
    group.addEventListener('dragleave', (e) => {
      if (!group.contains(e.relatedTarget)) {
        group.classList.remove('task-date-group-drop-target');
      }
    });
    group.addEventListener('drop', (e) => {
      e.preventDefault();
      const idx = parseInt(e.dataTransfer.getData('text/plain'), 10);
      if (Number.isNaN(idx) || !TASKS[idx]) return;
      if (TASKS[idx].date !== date) {
        TASKS[idx].date = date;
        renderTasksByDate();
      }
      group.classList.remove('task-date-group-drop-target');
    });
    iconsRefresh();
  });
}

function renderTodos(hoveredIdx) {
  const container = $('#task-todo-list');
  if (!container) return;
  container.innerHTML = '';

  TODOS.forEach((todo, i) => {
    const item = el('div', 'task-todo-item');
    if (i === hoveredIdx) item.classList.add('hovered');
    item.addEventListener('mouseleave', () => item.classList.remove('hovered'));
    item.innerHTML = `
      <div class="task-todo-check ${todo.done ? 'checked' : ''}" data-idx="${i}"></div>
      <span class="task-todo-text ${todo.done ? 'done' : ''}">${todo.text}</span>
      <div class="task-todo-trail">
        <span class="task-todo-meta">${todo.date}</span>
        <div class="task-todo-actions">
          <button class="icon-button task-todo-to-task" aria-label="轉成任務"><i data-lucide="arrow-right-to-line" class="icon"></i></button>
          <button class="icon-button task-todo-delete" aria-label="刪除"><i data-lucide="x" class="icon"></i></button>
        </div>
      </div>
    `;
    item.querySelector('.task-todo-check').addEventListener('click', () => {
      TODOS[i].done = !TODOS[i].done;
      renderTodos();
    });
    item.querySelector('.task-todo-delete').addEventListener('click', () => {
      confirmDialog({
        title: '確定要刪除此待辦？',
        desc: `「${TODOS[i].text}」將被移除，此動作無法復原。`,
        confirmText: '刪除',
        onConfirm: () => {
          TODOS.splice(i, 1);
          renderTodos();
        },
      });
    });
    item.querySelector('.task-todo-to-task').addEventListener('click', () => {
      openTaskModal();
      $('#task-f-name').value = todo.text;
    });
    // Single click to edit text
    function bindClickEdit(spanEl) {
      spanEl.addEventListener('click', () => {
        const input = document.createElement('input');
        input.className = 'task-todo-edit-input';
        input.value = TODOS[i].text;
        spanEl.replaceWith(input);
        input.focus();
        input.select();
        const commit = () => {
          const val = input.value.trim();
          if (val) TODOS[i].text = val;
          const newSpan = el('span', 'task-todo-text' + (TODOS[i].done ? ' done' : ''));
          newSpan.textContent = TODOS[i].text;
          input.replaceWith(newSpan);
          bindClickEdit(newSpan);
        };
        input.addEventListener('blur', commit);
        input.addEventListener('keydown', (e) => {
          if (e.key === 'Enter') input.blur();
          if (e.key === 'Escape') { input.value = TODOS[i].text; input.blur(); }
        });
      });
    }
    bindClickEdit(item.querySelector('.task-todo-text'));
    container.appendChild(item);
  });

  // Add new todo input
  const inputRow = el('div', 'task-todo-input-row');
  inputRow.innerHTML = `
    <div class="task-todo-check" style="border-style:dashed;opacity:0.5"></div>
    <input placeholder="新增待辦事項…">
  `;
  inputRow.querySelector('input').addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && e.target.value.trim()) {
      const today = new Date();
      TODOS.push({ text: e.target.value.trim(), done: false, date: `${String(today.getMonth()+1).padStart(2,'0')}/${String(today.getDate()).padStart(2,'0')}` });
      renderTodos();
    }
  });
  container.appendChild(inputRow);
  iconsRefresh();
}

let editingTaskIndex = null;

function openTaskModal(taskIndex) {
  const modal = $('#task-modal');
  if (!modal) return;
  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');

  const titleEl = $('#task-modal-title');
  const saveBtn = $('#task-modal-save');

  if (taskIndex != null) {
    // Edit mode
    editingTaskIndex = taskIndex;
    const t = TASKS[taskIndex];
    $('#task-f-name').value = t.name;
    $('#task-f-category').value = t.category;
    $('#task-f-hours').value = t.hours;
    $('#task-f-member').value = t.member;
    $('#task-f-date').value = t.date;
    $('#task-f-product').value = t.product;
    $('#task-f-note').value = '';
    if (titleEl) titleEl.textContent = '編輯任務';
    if (saveBtn) saveBtn.textContent = '更新';
  } else {
    // Create mode
    editingTaskIndex = null;
    $('#task-f-name').value = '';
    $('#task-f-category').value = '';
    $('#task-f-hours').value = '';
    $('#task-f-member').value = 'Tammy Chen';
    $('#task-f-product').value = '';
    $('#task-f-note').value = '';
    const today = new Date();
    $('#task-f-date').value = today.toISOString().slice(0, 10);
    if (titleEl) titleEl.textContent = '新增任務';
    if (saveBtn) saveBtn.textContent = '儲存';
  }
}

function closeTaskModal() {
  const modal = $('#task-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
  editingTaskIndex = null;
}

// Mobile-only tab switcher on the task page: splits the stacked view into
// two panes (打卡 · 待辦 / 任務紀錄) via a data attribute on .task-layout.
// Rearranges the task view DOM for mobile. Instead of using `display:
// contents` on wrapper divs (which is stripped from the a11y tree on iOS
// Safari < 14), we physically move elements so `.task-layout` becomes a
// flat flex column with every visible piece as a direct child. On
// desktop the original two-column structure is restored.
let taskLayoutMode = null;
function applyTaskLayoutForViewport() {
  const layout = document.querySelector('.task-layout');
  if (!layout) return;
  const wantMobile = window.matchMedia('(max-width: 720px)').matches;
  const targetMode = wantMobile ? 'mobile' : 'desktop';
  if (targetMode === taskLayoutMode) return;

  const taskLeft = layout.querySelector('.task-left');
  const taskRight = layout.querySelector('.task-right');
  const pageHead = layout.querySelector('.page-head');
  const taskRightDefault = layout.querySelector('.task-right-default');
  const taskRightAI = layout.querySelector('.task-right-ai');
  const tabs = layout.querySelector('.task-mobile-tabs');
  const monthNav = layout.querySelector('.task-month-nav');
  const dateList = layout.querySelector('#task-date-list');
  const newBtn = layout.querySelector('#task-new-btn');
  // Title container: the first div inside .page-head (holds the h2 + desc)
  const titleDiv = pageHead?.querySelector(':scope > div:not([id])');

  // Daily items — could be inside task-right-default OR flattened into layout
  const dailySelectors = ['.h4', '.home-clock-card', '.task-todo-list'];
  const dailyItems = () => {
    const inRightDefault = taskRightDefault
      ? [...taskRightDefault.children].filter(c => dailySelectors.some(s => c.matches(s)))
      : [];
    if (inRightDefault.length) return inRightDefault;
    return [...layout.children].filter(c => dailySelectors.some(s => c.matches(s)));
  };

  if (targetMode === 'mobile') {
    // Flatten into .task-layout as direct children in mobile reading order
    if (titleDiv) layout.appendChild(titleDiv);
    if (tabs) layout.appendChild(tabs);
    dailyItems().forEach(c => layout.appendChild(c));
    if (newBtn) layout.appendChild(newBtn);
    if (monthNav) layout.appendChild(monthNav);
    if (dateList) layout.appendChild(dateList);
    if (taskRightAI) layout.appendChild(taskRightAI);
  } else {
    // Restore desktop structure
    if (taskLeft && pageHead) {
      taskLeft.appendChild(pageHead);
      if (titleDiv) pageHead.insertBefore(titleDiv, pageHead.firstChild);
      if (newBtn) pageHead.appendChild(newBtn);
    }
    if (taskLeft && tabs) taskLeft.appendChild(tabs);
    if (taskLeft && monthNav) taskLeft.appendChild(monthNav);
    if (taskLeft && dateList) taskLeft.appendChild(dateList);
    if (taskRightDefault) {
      dailyItems().forEach(c => taskRightDefault.appendChild(c));
    }
    if (taskRight && taskRightDefault) taskRight.appendChild(taskRightDefault);
    if (taskRight && taskRightAI) taskRight.appendChild(taskRightAI);
  }
  taskLayoutMode = targetMode;
}

function initTaskMobileTabs() {
  const layout = $('.task-layout');
  const tablist = $('.task-mobile-tabs');
  const tabs = $$('.task-mobile-tabs .tab');
  if (!layout || !tabs.length) return;
  layout.setAttribute('data-mobile-tab', 'daily');
  tablist?.setAttribute('role', 'tablist');
  tabs.forEach((tab, i) => {
    const isActive = tab.classList.contains('active');
    tab.setAttribute('role', 'tab');
    tab.setAttribute('aria-selected', isActive ? 'true' : 'false');
    tab.setAttribute('tabindex', isActive ? '0' : '-1');
    tab.addEventListener('click', () => {
      tabs.forEach(t => {
        t.classList.remove('active');
        t.setAttribute('aria-selected', 'false');
        t.setAttribute('tabindex', '-1');
      });
      tab.classList.add('active');
      tab.setAttribute('aria-selected', 'true');
      tab.setAttribute('tabindex', '0');
      layout.setAttribute('data-mobile-tab', tab.getAttribute('data-mobile-tab'));
    });
    tab.addEventListener('keydown', (e) => {
      let target = null;
      if (e.key === 'ArrowRight' || e.key === 'ArrowDown') target = tabs[(i + 1) % tabs.length];
      else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') target = tabs[(i - 1 + tabs.length) % tabs.length];
      if (target) {
        e.preventDefault();
        target.click();
        target.focus();
      }
    });
  });
}

function initTaskModal() {
  // Init custom form components
  const catSelect = $('#task-f-category');
  if (catSelect) initCustomSelect(catSelect);
  const prodSelect = $('#task-f-product');
  if (prodSelect) initCustomSelect(prodSelect);
  const datePicker = $('#task-f-date');
  if (datePicker) initCustomDatepicker(datePicker);

  const newBtn = $('#task-new-btn');
  if (newBtn) newBtn.addEventListener('click', () => openTaskModal());

  const closeBtn = $('#task-modal-close');
  if (closeBtn) closeBtn.addEventListener('click', closeTaskModal);

  const cancelBtn = $('#task-modal-cancel');
  if (cancelBtn) cancelBtn.addEventListener('click', closeTaskModal);

  const saveBtn = $('#task-modal-save');
  if (saveBtn) saveBtn.addEventListener('click', () => {
    const name = $('#task-f-name').value.trim();
    if (!name) return;
    const data = {
      name,
      date: $('#task-f-date').value,
      member: $('#task-f-member').value,
      product: $('#task-f-product').value || 'Other',
      category: $('#task-f-category').value || 'Other',
      hours: parseFloat($('#task-f-hours').value) || 0,
    };
    if (editingTaskIndex != null) {
      TASKS[editingTaskIndex] = data;
    } else {
      TASKS.unshift(data);
    }
    closeTaskModal();
    renderTasksByDate();
  });

  // Close on overlay click
  const modal = $('#task-modal');
  if (modal) modal.addEventListener('click', (e) => {
    if (e.target === modal) closeTaskModal();
  });
}

// ---------- Dept Tasks Dashboard ----------
let deptViewYear = 2026;
let deptViewMonth = 4; // 1-based

function getDeptMonthTasks() {
  return TASKS.filter(t => {
    const [y, m] = t.date.split('-').map(Number);
    return y === deptViewYear && m === deptViewMonth;
  });
}

function updateDeptMonthLabel() {
  const label = $('#dept-month-label');
  if (label) label.textContent = `${deptViewYear} 年 ${deptViewMonth} 月`;
}

function shiftDeptMonth(delta) {
  const d = new Date(deptViewYear, (deptViewMonth - 1) + delta, 1);
  deptViewYear = d.getFullYear();
  deptViewMonth = d.getMonth() + 1;
  updateDeptMonthLabel();
  renderDeptTasksDashboard();
}

function initDeptMonthNav() {
  $('#dept-month-prev')?.addEventListener('click', () => shiftDeptMonth(-1));
  $('#dept-month-next')?.addEventListener('click', () => shiftDeptMonth(1));
  updateDeptMonthLabel();
}

const CLIENT_ROWS = [
  { name: '台達電子', owner: 'Tammy',  stage: '提案中', stageCls: 'badge-info',        amount: 'NT$320K' },
  { name: '鴻海精密', owner: 'Tammy',  stage: '報價中', stageCls: 'badge-warning',     amount: 'NT$580K' },
  { name: '聯發科技', owner: 'Kevin',  stage: '已成交', stageCls: 'badge-success',     amount: 'NT$210K' },
  { name: '華碩電腦', owner: 'Allen',  stage: '提案中', stageCls: 'badge-info',        amount: 'NT$150K' },
  { name: '緯創資通', owner: 'Tammy',  stage: '已流失', stageCls: 'badge-destructive', amount: 'NT$90K'  },
  { name: '廣達電腦', owner: 'Kevin',  stage: '報價中', stageCls: 'badge-warning',     amount: 'NT$440K' },
  { name: '仁寶電腦', owner: 'Allen',  stage: '提案中', stageCls: 'badge-info',        amount: 'NT$260K' },
  { name: '英業達',   owner: 'Tammy',  stage: '已成交', stageCls: 'badge-success',     amount: 'NT$380K' },
  { name: '微星科技', owner: 'Ella',   stage: '報價中', stageCls: 'badge-warning',     amount: 'NT$170K' },
  { name: '技嘉科技', owner: 'Kevin',  stage: '已成交', stageCls: 'badge-success',     amount: 'NT$290K' },
  { name: '友達光電', owner: 'Allen',  stage: '提案中', stageCls: 'badge-info',        amount: 'NT$230K' },
  { name: '群創光電', owner: 'Tammy',  stage: '已流失', stageCls: 'badge-destructive', amount: 'NT$110K' },
  { name: '台積電',   owner: 'Ella',   stage: '報價中', stageCls: 'badge-warning',     amount: 'NT$720K' },
  { name: '聯電',     owner: 'Kevin',  stage: '提案中', stageCls: 'badge-info',        amount: 'NT$340K' },
  { name: '日月光',   owner: 'Allen',  stage: '已成交', stageCls: 'badge-success',     amount: 'NT$500K' },
];

let clientPage = 1;
const CLIENT_PAGE_SIZE = 10;

function renderClientTable() {
  const body = $('#client-table-body');
  if (!body) return;
  const totalPages = Math.max(1, Math.ceil(CLIENT_ROWS.length / CLIENT_PAGE_SIZE));
  if (clientPage > totalPages) clientPage = totalPages;
  const start = (clientPage - 1) * CLIENT_PAGE_SIZE;
  const rows = CLIENT_ROWS.slice(start, start + CLIENT_PAGE_SIZE);
  body.innerHTML = rows.map(r => `
    <tr>
      <td>${r.name}</td>
      <td>
        <div class="client-owner">
          <span class="avatar">${r.owner.charAt(0)}</span>
          <span>${r.owner}</span>
        </div>
      </td>
      <td><span class="badge ${r.stageCls}">${r.stage}</span></td>
      <td>${r.amount}</td>
    </tr>
  `).join('');

  const host = $('#client-pagination');
  if (!host) return;
  if (totalPages <= 1) { host.innerHTML = ''; return; }
  host.innerHTML = `
    <div class="p-small muted">共 ${CLIENT_ROWS.length} 筆</div>
    <div class="pagination-pages">
      <button class="page-btn" data-dir="prev" ${clientPage === 1 ? 'disabled' : ''}>
        <i data-lucide="chevron-left" class="icon" style="width:14px;height:14px"></i>
      </button>
      ${Array.from({ length: totalPages }, (_, i) =>
        `<button class="page-btn ${i + 1 === clientPage ? 'active' : ''}" data-page="${i + 1}">${i + 1}</button>`
      ).join('')}
      <button class="page-btn" data-dir="next" ${clientPage === totalPages ? 'disabled' : ''}>
        <i data-lucide="chevron-right" class="icon" style="width:14px;height:14px"></i>
      </button>
    </div>
  `;
  host.querySelectorAll('.page-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const dir = btn.dataset.dir;
      if (dir === 'prev' && clientPage > 1) clientPage--;
      else if (dir === 'next' && clientPage < totalPages) clientPage++;
      else if (btn.dataset.page) clientPage = parseInt(btn.dataset.page, 10);
      renderClientTable();
      iconsRefresh();
    });
  });
}

function initSimpleMonthNav(prevId, nextId, labelId) {
  const prev = $('#' + prevId);
  const next = $('#' + nextId);
  const label = $('#' + labelId);
  if (!label) return;
  let year = 2026, month = 4;
  const render = () => { label.textContent = `${year} 年 ${month} 月`; };
  const shift = (delta) => {
    const d = new Date(year, (month - 1) + delta, 1);
    year = d.getFullYear();
    month = d.getMonth() + 1;
    render();
  };
  prev?.addEventListener('click', () => shift(-1));
  next?.addEventListener('click', () => shift(1));
  render();
}

function renderDeptTasksDashboard() {
  const monthTasks = getDeptMonthTasks();
  const totalHours = monthTasks.reduce((s, t) => s + t.hours, 0);
  const leaveDays = DEPT_LEAVES.reduce((s, l) => s + l.days, 0);
  const memberCount = TEAM_MEMBERS.length;

  const totalEl = $('#dept-total-hours');
  const avgEl = $('#dept-avg-hours');
  const leaveEl = $('#dept-leave-days');
  if (totalEl) totalEl.innerHTML = `${totalHours}<span class="p-small">h</span>`;
  if (avgEl) avgEl.innerHTML = `${(totalHours / memberCount).toFixed(1)}<span class="p-small">h</span>`;
  if (leaveEl) leaveEl.innerHTML = `${leaveDays}<span class="p-small">天</span>`;

  renderDeptLeaveChart();
  renderDeptHoursChart(monthTasks);
  renderDeptRatio('#dept-product-ratio', monthTasks, 'product');
  renderDeptRatio('#dept-category-ratio', monthTasks, 'category');
  renderDeptMemberTable(monthTasks);
  iconsRefresh();
}

function renderDeptLeaveChart() {
  const host = $('#dept-leave-chart');
  if (!host) return;
  const top = [...DEPT_LEAVES].sort((a, b) => b.days - a.days).slice(0, 5);
  const maxDays = Math.max(3, ...top.map(l => l.days));
  host.innerHTML = top.map(l => {
    const pct = l.days === 0 ? 0 : Math.max(6, (l.days / maxDays) * 100);
    const m = TEAM_MEMBERS.find(x => x.name === l.member);
    return `
      <div class="dept-bar-row">
        <div class="dept-bar-label">
          <span class="avatar">${m ? m.initial : '?'}</span>
          <span>${l.member}</span>
        </div>
        <div class="dept-bar-track">
          <div class="dept-bar-fill leave" style="transform:scaleX(${(pct/100).toFixed(4)})"></div>
        </div>
        <div class="dept-bar-value">${l.days} 天<span class="p-small muted"> · ${l.type}</span></div>
      </div>
    `;
  }).join('');
}

function renderDeptHoursChart(monthTasks) {
  const host = $('#dept-hours-chart');
  if (!host) return;
  const byMember = {};
  TEAM_MEMBERS.forEach(m => { byMember[m.name] = 0; });
  monthTasks.forEach(t => { byMember[t.member] = (byMember[t.member] || 0) + t.hours; });
  const top = [...TEAM_MEMBERS]
    .sort((a, b) => (byMember[b.name] || 0) - (byMember[a.name] || 0))
    .slice(0, 5);
  const maxHours = Math.max(1, ...top.map(m => byMember[m.name] || 0));
  host.innerHTML = top.map(m => {
    const hours = byMember[m.name] || 0;
    const pct = hours === 0 ? 0 : Math.max(6, (hours / maxHours) * 100);
    return `
      <div class="dept-bar-row">
        <div class="dept-bar-label">
          <span class="avatar">${m.initial}</span>
          <span>${m.name}</span>
        </div>
        <div class="dept-bar-track">
          <div class="dept-bar-fill hours" style="transform:scaleX(${(pct/100).toFixed(4)})"></div>
        </div>
        <div class="dept-bar-value">${hours}<span class="p-small muted">h</span></div>
      </div>
    `;
  }).join('');
}

const DEPT_RATIO_PALETTE = ['#4f46e5', '#06b6d4', '#f59e0b', '#ec4899', '#10b981', '#8b5cf6', '#ef4444'];

function renderDeptRatio(selector, monthTasks, key) {
  const host = $(selector);
  if (!host) return;
  const totals = {};
  monthTasks.forEach(t => { totals[t[key]] = (totals[t[key]] || 0) + t.hours; });
  const entries = Object.entries(totals).sort((a, b) => b[1] - a[1]);
  const sum = entries.reduce((s, [, v]) => s + v, 0) || 1;

  const segments = entries.map(([name, v], i) => {
    const pct = (v / sum) * 100;
    return `<div class="dept-ratio-seg" style="width:${pct}%;background:${DEPT_RATIO_PALETTE[i % DEPT_RATIO_PALETTE.length]}" title="${name} ${pct.toFixed(1)}%"></div>`;
  }).join('');

  const items = entries.map(([name, v], i) => {
    const pct = (v / sum) * 100;
    return `
      <div class="dept-ratio-item">
        <span class="dept-ratio-dot" style="background:${DEPT_RATIO_PALETTE[i % DEPT_RATIO_PALETTE.length]}"></span>
        <span class="dept-ratio-name">${name}</span>
        <span class="dept-ratio-meta p-small muted">${v}h · ${pct.toFixed(1)}%</span>
      </div>
    `;
  }).join('');

  host.innerHTML = `
    <div class="dept-ratio-bar">${segments}</div>
    <div class="dept-ratio-legend">${items}</div>
  `;
}

let deptMemberPage = 1;
const DEPT_MEMBER_PAGE_SIZE = 10;
let deptSortKey = null;   // 'hours' | 'leave' | null
let deptSortDir = 'desc'; // 'asc' | 'desc'

function deptMemberRowData(m, monthTasks) {
  const mine = monthTasks.filter(t => t.member === m.name);
  const hours = mine.reduce((s, t) => s + t.hours, 0);
  const leave = DEPT_LEAVES.find(l => l.member === m.name);
  return { member: m, hours, leaveDays: leave ? leave.days : 0, leave };
}

function renderDeptMemberTable(monthTasks) {
  const body = $('#dept-member-body');
  if (!body) return;


  // Pre-compute per-member data so we can sort reliably
  let rows = TEAM_MEMBERS.map(m => deptMemberRowData(m, monthTasks));
  if (deptSortKey === 'hours') {
    rows.sort((a, b) => deptSortDir === 'asc' ? a.hours - b.hours : b.hours - a.hours);
  } else if (deptSortKey === 'leave') {
    rows.sort((a, b) => deptSortDir === 'asc' ? a.leaveDays - b.leaveDays : b.leaveDays - a.leaveDays);
  }

  // Reflect sort state on headers
  $$('.dept-member-table th.sortable').forEach(th => {
    th.classList.remove('sort-asc', 'sort-desc');
    if (th.dataset.sort === deptSortKey) {
      th.classList.add(deptSortDir === 'asc' ? 'sort-asc' : 'sort-desc');
    }
  });

  const totalPages = Math.max(1, Math.ceil(rows.length / DEPT_MEMBER_PAGE_SIZE));
  if (deptMemberPage > totalPages) deptMemberPage = totalPages;
  const start = (deptMemberPage - 1) * DEPT_MEMBER_PAGE_SIZE;
  const pageItems = rows.slice(start, start + DEPT_MEMBER_PAGE_SIZE).map(r => r.member);

  body.innerHTML = pageItems.map(m => {
    const mine = monthTasks.filter(t => t.member === m.name);
    const hours = mine.reduce((s, t) => s + t.hours, 0);
    const productCounts = {};
    mine.forEach(t => { productCounts[t.product] = (productCounts[t.product] || 0) + t.hours; });
    const topProduct = Object.entries(productCounts).sort((a, b) => b[1] - a[1])[0];
    const leave = DEPT_LEAVES.find(l => l.member === m.name);
    return `
      <tr>
        <td>
          <div class="dept-member-cell">
            <span class="avatar">${m.initial}</span>
            <span>${m.name}</span>
          </div>
        </td>
        <td>${hours}h</td>
        <td>${leave && leave.days > 0 ? `${leave.days} 天 <span class="p-small muted">${leave.type}</span>` : '—'}</td>
        <td>${topProduct ? `<span class="badge badge-secondary">${topProduct[0]}</span>` : '—'}</td>
        <td>
          <button class="btn btn-outline btn-sm dept-view-btn" data-member="${m.name}">
            <i data-lucide="eye" class="icon"></i> 檢視
          </button>
        </td>
      </tr>
    `;
  }).join('');

  body.querySelectorAll('.dept-view-btn').forEach(btn => {
    btn.addEventListener('click', () => openDeptMemberModal(btn.dataset.member));
  });

  renderDeptMemberPagination(totalPages, monthTasks);
}

function renderDeptMemberPagination(totalPages, monthTasks) {
  const host = $('#dept-member-pagination');
  if (!host) return;
  if (totalPages <= 1) { host.innerHTML = ''; return; }
  const btnPage = (n, active = false) =>
    `<button class="page-btn ${active ? 'active' : ''}" data-page="${n}">${n}</button>`;
  host.innerHTML = `
    <div class="p-small muted">共 ${TEAM_MEMBERS.length} 人</div>
    <div class="pagination-pages">
      <button class="page-btn" data-dir="prev" ${deptMemberPage === 1 ? 'disabled' : ''}>
        <i data-lucide="chevron-left" class="icon" style="width:14px;height:14px"></i>
      </button>
      ${Array.from({ length: totalPages }, (_, i) => btnPage(i + 1, i + 1 === deptMemberPage)).join('')}
      <button class="page-btn" data-dir="next" ${deptMemberPage === totalPages ? 'disabled' : ''}>
        <i data-lucide="chevron-right" class="icon" style="width:14px;height:14px"></i>
      </button>
    </div>
  `;
  host.querySelectorAll('.page-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const dir = btn.dataset.dir;
      if (dir === 'prev' && deptMemberPage > 1) deptMemberPage--;
      else if (dir === 'next' && deptMemberPage < totalPages) deptMemberPage++;
      else if (btn.dataset.page) deptMemberPage = parseInt(btn.dataset.page, 10);
      renderDeptMemberTable(monthTasks);
      iconsRefresh();
    });
  });
}

function openDeptMemberModal(memberName) {
  const modal = $('#dept-member-modal');
  if (!modal) return;
  const mine = getDeptMonthTasks().filter(t => t.member === memberName);
  const hours = mine.reduce((s, t) => s + t.hours, 0);
  const products = new Set(mine.map(t => t.product));
  const leave = DEPT_LEAVES.find(l => l.member === memberName);

  const title = $('#dept-member-modal-title');
  if (title) title.textContent = `${memberName} · 工作任務`;
  const sub = $('#dept-member-modal-sub');
  if (sub) sub.textContent = `${deptViewYear} 年 ${deptViewMonth} 月`;
  const hoursEl = $('#dept-member-stat-hours');
  const tasksEl = $('#dept-member-stat-tasks');
  const productsEl = $('#dept-member-stat-products');
  const leaveEl = $('#dept-member-stat-leave');
  if (hoursEl) hoursEl.innerHTML = `${hours}<span class="p-small">h</span>`;
  if (tasksEl) tasksEl.textContent = mine.length;
  if (productsEl) productsEl.textContent = products.size;
  if (leaveEl) leaveEl.innerHTML = `${leave ? leave.days : 0}<span class="p-small">天</span>`;

  const body = $('#dept-member-task-body');
  if (body) {
    const sorted = [...mine].sort((a, b) => b.date.localeCompare(a.date));
    body.innerHTML = sorted.length === 0
      ? `<tr><td colspan="5" style="text-align:center;color:var(--muted-foreground);padding:var(--space-xl)">本月無任務紀錄</td></tr>`
      : sorted.map(t => {
          const d = new Date(t.date + 'T00:00:00');
          const label = `${d.getMonth()+1}/${d.getDate()}`;
          return `
            <tr>
              <td>${label}</td>
              <td>${t.name}</td>
              <td><span class="badge badge-secondary">${t.product}</span></td>
              <td><span class="badge badge-secondary">${t.category}</span></td>
              <td>${t.hours}h</td>
            </tr>
          `;
        }).join('');
  }

  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  iconsRefresh();
}

function initDeptMemberSort() {
  $$('.dept-member-table th.sortable').forEach(th => {
    th.addEventListener('click', () => {
      const key = th.dataset.sort;
      if (deptSortKey === key) {
        deptSortDir = deptSortDir === 'asc' ? 'desc' : 'asc';
      } else {
        deptSortKey = key;
        deptSortDir = 'desc';
      }
      deptMemberPage = 1;
      renderDeptMemberTable(getDeptMonthTasks());
      iconsRefresh();
    });
  });
}

function initDeptMemberModal() {
  const modal = $('#dept-member-modal');
  if (!modal) return;
  const close = () => {
    modal.classList.remove('open');
    modal.setAttribute('aria-hidden', 'true');
  };
  $('#dept-member-modal-close')?.addEventListener('click', close);
  modal.addEventListener('click', (e) => { if (e.target === modal) close(); });
}

// ---------- Attendance modal month nav ----------
let attYear = 2026;
let attMonth = 4; // 1-based

const MONTHLY_STATS = {
  '2026-4': { days: 12, hours: 96, leave: 1, overtime: 3 },
  '2026-3': { days: 22, hours: 176, leave: 0, overtime: 8 },
  '2026-2': { days: 18, hours: 144, leave: 2, overtime: 4 },
  '2026-1': { days: 20, hours: 160, leave: 1, overtime: 6 },
  '2025-12': { days: 21, hours: 168, leave: 0, overtime: 5 },
};

function updateAttendanceMonth() {
  const label = $('#att-month-label');
  if (label) label.textContent = `${attYear} 年 ${attMonth} 月`;

  const key = `${attYear}-${attMonth}`;
  const stats = MONTHLY_STATS[key] || { days: 0, hours: 0, leave: 0, overtime: 0 };

  const daysEl = $('#att-stat-days');
  const hoursEl = $('#att-stat-hours');
  const leaveEl = $('#att-stat-leave');
  const overtimeEl = $('#att-stat-overtime');
  if (daysEl) daysEl.textContent = stats.days;
  if (hoursEl) hoursEl.innerHTML = `${stats.hours}<span class="p-small">h</span>`;
  if (leaveEl) leaveEl.textContent = stats.leave;
  if (overtimeEl) overtimeEl.innerHTML = `${stats.hours > 0 ? stats.overtime : 0}<span class="p-small">h</span>`;

  // Disable next if current month
  const now = new Date();
  const nextBtn = $('#att-month-next');
  if (nextBtn) nextBtn.disabled = (attYear === now.getFullYear() && attMonth === now.getMonth() + 1);

  // Render monthly punch table
  renderMonthlyPunch();
}

function renderMonthlyPunch() {
  const tbody = $('#att-month-body');
  if (!tbody) return;

  const now = new Date();
  const dayNames = ['日','一','二','三','四','五','六'];
  const statusMap = {
    working:  { badge: 'badge-info',        text: '工作中' },
    normal:   { badge: 'badge-success',     text: '正常' },
    overtime: { badge: 'badge-warning',     text: '加班' },
    leave:    { badge: 'badge-ghost',       text: '請假' },
    weekend:  { badge: 'badge-secondary',   text: '假日' },
    missed:   { badge: 'badge-destructive', text: '需補卡' },
    future:   { badge: 'badge-secondary',   text: '—' },
  };

  const daysInMonth = new Date(attYear, attMonth, 0).getDate();
  let html = '';

  for (let d = 1; d <= daysInMonth; d++) {
    const date = new Date(attYear, attMonth - 1, d);
    const dow = date.getDay();
    const isWeekend = (dow === 0 || dow === 6);
    const isFuture = date > now;
    const punchKey = `${attYear}/${attMonth}/${d}`;

    let rec = PUNCH_RECORDS[punchKey];
    if (!rec) {
      if (isFuture) rec = { in: null, out: null, hours: null, status: 'future' };
      else if (isWeekend) rec = { in: null, out: null, hours: null, status: 'weekend' };
      else rec = { in: null, out: null, hours: null, status: 'missed' };
    }

    const st = statusMap[rec.status] || statusMap.future;
    const dateLabel = `${attYear}/${String(attMonth).padStart(2,'0')}/${String(d).padStart(2,'0')}（${dayNames[dow]}）`;

    const isHoliday = isWeekend || rec.status === 'weekend';
    html += `<tr class="${isHoliday ? 'dash-row-weekend' : ''}">
      <td>${dateLabel}</td>
      <td>${rec.in || '—'}</td>
      <td>${rec.out || '—'}</td>
      <td>${rec.hours ? rec.hours + 'h' : '—'}</td>
      <td><span class="badge ${st.badge}">${st.text}</span></td>
      <td>${rec.status === 'missed' ? '<a class="link-btn p-small">填寫</a>' : ''}</td>
    </tr>`;
  }
  tbody.innerHTML = html;
}

function initAttendanceMonthNav() {
  $('#att-month-prev')?.addEventListener('click', () => {
    attMonth--;
    if (attMonth < 1) { attMonth = 12; attYear--; }
    updateAttendanceMonth();
  });
  $('#att-month-next')?.addEventListener('click', () => {
    const now = new Date();
    if (attYear === now.getFullYear() && attMonth === now.getMonth() + 1) return;
    attMonth++;
    if (attMonth > 12) { attMonth = 1; attYear++; }
    updateAttendanceMonth();
  });
}

// ---------- Render: Home ----------
function renderAssistantScroller() {
  const scroller = $('#assistant-scroller');
  if (!scroller) return;
  scroller.innerHTML = '';
  ASSISTANTS.slice(0, 12).forEach(a => {
    const card = el('div', 'assistant-card');
    card.innerHTML = `
      <div class="assistant-emoji">${a.emoji}</div>
      <div class="assistant-content">
        <div class="assistant-title">${a.title}</div>
        <div class="assistant-desc">${a.desc}</div>
      </div>
    `;
    card.addEventListener('click', () => switchView('chat', { assistant: a }));
    scroller.appendChild(card);
  });

  const leftBtn = $('#assist-scroll-left');
  const rightBtn = $('#assist-scroll-right');
  // Force initial hidden state (can't scroll left at scrollLeft = 0)
  if (leftBtn) leftBtn.hidden = true;
  const updateArrows = () => {
    const maxScroll = Math.max(0, scroller.scrollWidth - scroller.clientWidth);
    const x = scroller.scrollLeft;
    const atStart = x <= 2;
    const atEnd = maxScroll <= 0 || x >= maxScroll - 2;
    if (leftBtn)  leftBtn.hidden  = atStart;
    if (rightBtn) rightBtn.hidden = atEnd;
  };

  leftBtn?.addEventListener('click', () => {
    scroller.scrollBy({ left: -scroller.clientWidth * 0.8, behavior: 'smooth' });
  });
  rightBtn?.addEventListener('click', () => {
    scroller.scrollBy({ left: scroller.clientWidth * 0.8, behavior: 'smooth' });
  });
  scroller.addEventListener('scroll', updateArrows, { passive: true });
  window.addEventListener('resize', updateArrows);
  // Initial state (wait a tick so layout is complete)
  requestAnimationFrame(updateArrows);
}

function renderFormsGrid() {
  const grid = $('#forms-grid');
  grid.innerHTML = '';
  FORM_COLUMNS.forEach(col => {
    const column = el('div', 'form-column');
    column.innerHTML = `
      <div class="form-column-head">
        <div class="p-medium">${col.title}</div>
      </div>
      <div class="form-list">
        ${col.items.map(it => `
          <div class="form-item" data-title="${it.title}">
            <div class="form-item-main">
              <span class="form-item-emoji">${it.emoji}</span>
              <div class="form-item-text">
                <div class="form-item-title">${it.title}</div>
                <div class="form-item-desc">${it.desc}</div>
              </div>
            </div>
            <i data-lucide="chevron-right" class="chevron"></i>
          </div>
        `).join('')}
      </div>
    `;
    grid.appendChild(column);
  });

  // Forms grid items on home → open form modal directly
  $$('.form-item').forEach(item => {
    item.addEventListener('click', () => {
      const title = item.getAttribute('data-title') || item.querySelector('.form-item-title')?.textContent;
      openFormModal(title);
    });
  });
}

// ---------- Render: Notifications ----------
const NOTIF_EMOJI_BY_TITLE = {
  '請假申請單': '🗓️',
  '加班核准申請單': '⏰',
  '加班申請單': '⏰',
  '出差申請單': '✈️',
  '補卡申請單': '🕒',
  '外勤申請單': '📍',
  '離職/退休申請': '👋',
  '費用報銷單': '💳',
  '付款申請單': '💵',
  '採購單申請': '🛒',
  '物資領用單': '📦',
  '用印申請單': '🔖',
  '資產報修申請單': '🔧',
  '帳號申請單': '🔐',
  '通用簽呈': '📝',
};

const NOTIF_EMOJI_BY_STATUS = {
  success: '✅',
  warning: '⚠️',
  info: '📢',
  destructive: '❌',
  secondary: '📬',
};

function buildNotifItem(n, variantOrOpts = 'form') {
  const opts = typeof variantOrOpts === 'string' ? { variant: variantOrOpts } : variantOrOpts;
  const { variant = 'form', showCheck = true } = opts;

  const badgeClass =
    n.status === 'success'     ? 'badge-success' :
    n.status === 'warning'     ? 'badge-warning' :
    n.status === 'info'        ? 'badge-info'    :
    n.status === 'destructive' ? 'badge-destructive' : 'badge-secondary';

  const emoji = NOTIF_EMOJI_BY_TITLE[n.title] || NOTIF_EMOJI_BY_STATUS[n.status] || '📄';

  // System notifications (header bell dropdown / all-notifs modal) keep the
  // card-style layout — they're not part of the table-style list views.
  if (variant === 'system') {
    const item = el('div', 'notif-item');
    item.innerHTML = `
      <div class="notif-row">
        <span class="badge ${badgeClass}">${n.statusText}</span>
        <span class="notif-meta">${fmtDateLong(n.time)}</span>
      </div>
      <div class="notif-title">${n.title}</div>
      <div class="notif-desc">${n.who ? n.who + ' · ' : ''}${n.desc}</div>
    `;
    return item;
  }

  // Form-list table row — [checkbox?] [status] [content] [time] [chevron]
  const row = el('tr', 'notif-row-tr');
  const checkCell = showCheck
    ? `<td class="emp-check-td"><span class="notif-item-select bulk-review-check" role="checkbox" aria-checked="false" tabindex="0" aria-label="選取此項目"></span></td>`
    : '';
  row.innerHTML = `
    ${checkCell}
    <td class="notif-status-cell"><span class="badge ${badgeClass}">${n.statusText}</span></td>
    <td class="notif-content-cell">
      <div class="form-draft-main">
        <span class="form-draft-emoji">${emoji}</span>
        <div class="form-draft-text">
          <div class="form-draft-title-row">
            <span class="form-draft-title">${n.title}</span>
            <span class="badge ${badgeClass} notif-badge-inline">${n.statusText}</span>
          </div>
          <div class="form-draft-summary">${n.who ? n.who + ' · ' : ''}${n.desc}</div>
          <div class="form-draft-time-inline">${fmtDateLong(n.time)}</div>
        </div>
      </div>
    </td>
    <td class="notif-time-cell p-small muted">${fmtDateLong(n.time)}</td>
    <td class="notif-chevron-cell"><i data-lucide="chevron-right" class="icon"></i></td>
  `;
  return row;
}

let currentReviewedStatus = 'all';
let currentReviewedDateRange = 'all';

function parseReviewedDate(str) {
  // str like "4/8 16:22" — assume current year
  const datePart = (str || '').trim().split(' ')[0];
  const [m, d] = datePart.split('/').map(Number);
  if (!m || !d) return null;
  return new Date(new Date().getFullYear(), m - 1, d);
}

function isReviewedInDateRange(dateStr, range) {
  if (range === 'all') return true;
  const d = parseReviewedDate(dateStr);
  if (!d) return true;
  const now = new Date();
  now.setHours(23, 59, 59, 999);
  switch (range) {
    case '7d': {
      const since = new Date(now);
      since.setDate(since.getDate() - 7);
      return d >= since && d <= now;
    }
    case '30d': {
      const since = new Date(now);
      since.setDate(since.getDate() - 30);
      return d >= since && d <= now;
    }
    case 'thisMonth':
      return d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth();
    case 'lastMonth': {
      const lm = new Date(now.getFullYear(), now.getMonth() - 1, 1);
      return d.getFullYear() === lm.getFullYear() && d.getMonth() === lm.getMonth();
    }
    default:
      return true;
  }
}

function filterReviewed(status = currentReviewedStatus, range = currentReviewedDateRange) {
  let items = REVIEWED;
  if (status === 'approved')      items = items.filter(r => r.status === 'success');
  else if (status === 'rejected') items = items.filter(r => r.status === 'destructive' || r.status === 'secondary');
  if (range !== 'all')            items = items.filter(r => isReviewedInDateRange(r.time, range));
  return items;
}

function updateReviewedFilterCounts() {
  const counts = {
    all:      filterReviewed('all',      currentReviewedDateRange).length,
    approved: filterReviewed('approved', currentReviewedDateRange).length,
    rejected: filterReviewed('rejected', currentReviewedDateRange).length,
  };
  Object.entries(counts).forEach(([key, val]) => {
    const el = document.querySelector(`#reviewed-filter-dropdown [data-count="${key}"]`);
    if (el) el.textContent = val;
  });
}

let currentPendingDateRange = 'all';
let currentPendingType = 'all';

function parseRelativeTime(str) {
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const s = (str || '').trim();
  if (!s) return null;
  if (s.startsWith('今天')) return today;
  if (s.startsWith('昨天')) {
    const d = new Date(today);
    d.setDate(d.getDate() - 1);
    return d;
  }
  // Absolute like "4/13" or "4/13 16:22"
  const datePart = s.split(' ')[0];
  const [m, dd] = datePart.split('/').map(Number);
  if (!m || !dd) return null;
  return new Date(today.getFullYear(), m - 1, dd);
}

function isRelativeInDateRange(str, range) {
  if (range === 'all') return true;
  const d = parseRelativeTime(str);
  if (!d) return true;
  const now = new Date();
  now.setHours(23, 59, 59, 999);
  switch (range) {
    case '7d': {
      const since = new Date(now);
      since.setDate(since.getDate() - 7);
      return d >= since && d <= now;
    }
    case '30d': {
      const since = new Date(now);
      since.setDate(since.getDate() - 30);
      return d >= since && d <= now;
    }
    case 'thisMonth':
      return d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth();
    case 'lastMonth': {
      const lm = new Date(now.getFullYear(), now.getMonth() - 1, 1);
      return d.getFullYear() === lm.getFullYear() && d.getMonth() === lm.getMonth();
    }
    default:
      return true;
  }
}

let pendingPage = 1;
let reviewedPage = 1;
let notifiedPage = 1;
let currentNotifiedDateRange = 'all';
// Bulk review: tracks which PENDING items the user has checked
const bulkReviewSelected = new Set();

function renderNotifications() {
  const pending  = $('#pending-list');
  const reviewed = $('#reviewed-list');
  const notified = $('#notified-list');
  const system   = $('#system-list');
  pending.innerHTML  = '';
  reviewed.innerHTML = '';
  if (notified) notified.innerHTML = '';
  system.innerHTML   = '';

  const selectableSelector =
    '#pending-list .notif-item, #reviewed-list .notif-item, #notified-list .notif-item, #system-list .notif-item';

  const pendingItems = PENDING.filter(n =>
    isRelativeInDateRange(n.time, currentPendingDateRange) &&
    (currentPendingType === 'all' || n.title === currentPendingType)
  );
  const pendingTotalPages = Math.max(1, Math.ceil(pendingItems.length / LIST_PAGE_SIZE));
  if (pendingPage > pendingTotalPages) pendingPage = pendingTotalPages;
  const pendingPageItems = pendingItems.slice((pendingPage - 1) * LIST_PAGE_SIZE, pendingPage * LIST_PAGE_SIZE);
  if (pendingItems.length === 0) {
    pending.innerHTML = `
      <tr><td colspan="5"><div class="empty-state">
        <i data-lucide="inbox" class="icon"></i>
        <div class="h4">此條件下沒有待處理項目</div>
      </div></td></tr>
    `;
    // Clear any stale selection that pointed at the now-filtered-out items
    bulkReviewSelected.clear();
  } else {
    pendingPageItems.forEach((n) => {
      const it = buildNotifItem(n);
      const check = it.querySelector('.notif-item-select');
      // Reflect any prior selection state
      if (bulkReviewSelected.has(n)) {
        check?.setAttribute('aria-checked', 'true');
        it.classList.add('bulk-selected');
      }
      check?.addEventListener('click', (ev) => {
        ev.stopPropagation();
        toggleBulkReviewItem(n, it);
      });
      check?.addEventListener('keydown', (ev) => {
        if (ev.key === ' ' || ev.key === 'Enter') {
          ev.preventDefault();
          ev.stopPropagation();
          toggleBulkReviewItem(n, it);
        }
      });
      it.addEventListener('click', () => {
        $$(selectableSelector).forEach(x => x.classList.remove('selected'));
        it.classList.add('selected');
        // Kick off a review session starting at this item so clicking
        // review actions auto-advances through the remaining pending queue.
        startReviewSession(n);
        openFormModal(n.title, 'review', n);
      });
      pending.appendChild(it);
    });
  }
  updateBulkReviewUI(pendingItems.length === 0 ? [] : pendingPageItems);
  renderPagination({
    hostId: 'pending-pagination',
    totalItems: pendingItems.length,
    page: pendingPage,
    pageSize: LIST_PAGE_SIZE,
    onPageChange: (p) => { pendingPage = p; renderNotifications(); },
  });

  const reviewedItems = filterReviewed();
  const reviewedTotalPages = Math.max(1, Math.ceil(reviewedItems.length / LIST_PAGE_SIZE));
  if (reviewedPage > reviewedTotalPages) reviewedPage = reviewedTotalPages;
  const reviewedPageItems = reviewedItems.slice((reviewedPage - 1) * LIST_PAGE_SIZE, reviewedPage * LIST_PAGE_SIZE);
  if (reviewedItems.length === 0) {
    reviewed.innerHTML = `
      <tr><td colspan="4"><div class="empty-state">
        <i data-lucide="clipboard-list" class="icon"></i>
        <div class="h4">此條件下沒有已審核紀錄</div>
      </div></td></tr>
    `;
  } else {
    reviewedPageItems.forEach(n => {
      const it = buildNotifItem(n, { showCheck: false });
      it.addEventListener('click', () => {
        $$(selectableSelector).forEach(x => x.classList.remove('selected'));
        it.classList.add('selected');
        openFormModal(n.title, 'reviewed', n);
      });
      reviewed.appendChild(it);
    });
  }
  renderPagination({
    hostId: 'reviewed-pagination',
    totalItems: reviewedItems.length,
    page: reviewedPage,
    pageSize: LIST_PAGE_SIZE,
    onPageChange: (p) => { reviewedPage = p; renderNotifications(); },
  });
  updateReviewedFilterCounts();

  // ----- 已知會 list -----
  if (notified) {
    const notifiedItems = NOTIFIED.filter(n => isRelativeInDateRange(n.time, currentNotifiedDateRange));
    const notifiedTotalPages = Math.max(1, Math.ceil(notifiedItems.length / LIST_PAGE_SIZE));
    if (notifiedPage > notifiedTotalPages) notifiedPage = notifiedTotalPages;
    const notifiedPageItems = notifiedItems.slice((notifiedPage - 1) * LIST_PAGE_SIZE, notifiedPage * LIST_PAGE_SIZE);
    if (notifiedItems.length === 0) {
      notified.innerHTML = `
        <tr><td colspan="4"><div class="empty-state">
          <i data-lucide="bell-off" class="icon"></i>
          <div class="h4">此條件下沒有知會紀錄</div>
        </div></td></tr>
      `;
    } else {
      notifiedPageItems.forEach(n => {
        const it = buildNotifItem(n, { showCheck: false });
        it.addEventListener('click', () => {
          $$(selectableSelector).forEach(x => x.classList.remove('selected'));
          it.classList.add('selected');
          openFormModal(n.title, 'reviewed', n);
        });
        notified.appendChild(it);
      });
    }
    renderPagination({
      hostId: 'notified-pagination',
      totalItems: notifiedItems.length,
      page: notifiedPage,
      pageSize: LIST_PAGE_SIZE,
      onPageChange: (p) => { notifiedPage = p; renderNotifications(); },
    });
  }

  SYSTEM_NOTIFS.forEach(n => {
    const it = buildNotifItem(n, 'system');
    it.addEventListener('click', () => {
      $$(selectableSelector).forEach(x => x.classList.remove('selected'));
      it.classList.add('selected');
    });
    system.appendChild(it);
  });

  // Keep tab counts in sync with data
  const pc = $('#pending-count');
  const rc = $('#reviewed-count');
  const nc = $('#notified-count');
  if (pc) pc.textContent = PENDING.length;
  if (rc) rc.textContent = REVIEWED.length;
  if (nc) nc.textContent = NOTIFIED.length;

  // Refresh any lucide icons rendered into this pass (empty-state inbox,
  // notif cards, etc.). Without this, re-renders triggered by bulk-review
  // toggles leave raw <i data-lucide> markers in the DOM instead of SVGs.
  iconsRefresh();
}

// ---------- Bulk review ----------
function toggleBulkReviewItem(n, itemEl) {
  if (bulkReviewSelected.has(n)) {
    bulkReviewSelected.delete(n);
    itemEl.classList.remove('bulk-selected');
    itemEl.querySelector('.notif-item-select')?.setAttribute('aria-checked', 'false');
  } else {
    bulkReviewSelected.add(n);
    itemEl.classList.add('bulk-selected');
    itemEl.querySelector('.notif-item-select')?.setAttribute('aria-checked', 'true');
  }
  const pendingItems = PENDING.filter(p =>
    isRelativeInDateRange(p.time, currentPendingDateRange) &&
    (currentPendingType === 'all' || p.title === currentPendingType)
  );
  const pageItems = pendingItems.slice((pendingPage - 1) * LIST_PAGE_SIZE, pendingPage * LIST_PAGE_SIZE);
  updateBulkReviewUI(pageItems);
}

function updateBulkReviewUI(pageItems) {
  const count = bulkReviewSelected.size;
  const actionbar = $('#bulk-review-actionbar');
  const countEl = $('#bulk-review-count');
  const summary = $('#bulk-review-summary');
  const selectAll = $('#bulk-review-selectall-check');
  const selectAllLabel = $('#bulk-review-selectall-label');

  if (countEl) countEl.textContent = count;
  if (actionbar) actionbar.hidden = count === 0;

  // Select-all tri-state: none checked → "", all-on-page checked → true, some → mixed
  const pageSelectedCount = pageItems.filter(n => bulkReviewSelected.has(n)).length;
  if (selectAll) {
    if (pageSelectedCount === 0) selectAll.setAttribute('aria-checked', 'false');
    else if (pageSelectedCount === pageItems.length) selectAll.setAttribute('aria-checked', 'true');
    else selectAll.setAttribute('aria-checked', 'mixed');
  }
  if (selectAllLabel) {
    selectAllLabel.textContent = count > 0 ? `已選 ${count} 件` : '全選';
  }
  if (summary) {
    summary.textContent = pageItems.length > 0 ? `共 ${pageItems.length} 件` : '';
  }
}

function initBulkReview() {
  const selectAll = $('#bulk-review-selectall-check');
  const selectAllLabel = $('#bulk-review-selectall-label')?.closest('.bulk-review-selectall');
  const toggleSelectAll = () => {
    const pendingItems = PENDING.filter(p =>
    isRelativeInDateRange(p.time, currentPendingDateRange) &&
    (currentPendingType === 'all' || p.title === currentPendingType)
  );
    const pageItems = pendingItems.slice((pendingPage - 1) * LIST_PAGE_SIZE, pendingPage * LIST_PAGE_SIZE);
    const allSelected = pageItems.length > 0 && pageItems.every(n => bulkReviewSelected.has(n));
    pageItems.forEach(n => {
      if (allSelected) bulkReviewSelected.delete(n);
      else bulkReviewSelected.add(n);
    });
    renderNotifications();
  };
  selectAll?.addEventListener('click', toggleSelectAll);
  selectAllLabel?.addEventListener('click', (e) => {
    if (e.target === selectAll) return;
    toggleSelectAll();
  });
  selectAll?.addEventListener('keydown', (e) => {
    if (e.key === ' ' || e.key === 'Enter') {
      e.preventDefault();
      toggleSelectAll();
    }
  });

  $('#bulk-review-cancel')?.addEventListener('click', () => {
    bulkReviewSelected.clear();
    renderNotifications();
  });

  const applyBulkAction = (action) => {
    const count = bulkReviewSelected.size;
    if (count === 0) return;
    const label = action === 'approve' ? '核准' : action === 'reject' ? '不通過' : '退回補件';
    confirmDialog({
      title: `確定要批次${label} ${count} 件？`,
      desc: '送出後 6 秒內可點「還原」撤回。',
      confirmText: `確定${label}`,
      onConfirm: () => {
        // Snapshot the removed items with their original PENDING index so
        // we can restore them on undo. Sort the snapshot ascending by index
        // for a clean re-insert in reverse order (rightmost first) that
        // keeps every earlier index valid.
        const snapshot = [...bulkReviewSelected]
          .map(n => ({ item: n, index: PENDING.indexOf(n) }))
          .filter(x => x.index >= 0)
          .sort((a, b) => a.index - b.index);

        snapshot.slice().reverse().forEach(({ index }) => PENDING.splice(index, 1));
        bulkReviewSelected.clear();
        renderNotifications();

        showToast({
          title: `已${label} ${count} 件`,
          desc: '批次處理完成',
          variant: action === 'approve' ? 'success' : 'info',
          action: {
            label: '還原',
            onClick: () => {
              // Re-insert in original-index order so items land back in place
              snapshot.forEach(({ item, index }) => {
                const insertAt = Math.min(index, PENDING.length);
                PENDING.splice(insertAt, 0, item);
              });
              renderNotifications();
              showToast({
                title: '已還原',
                desc: `${count} 件已重新回到待處理清單`,
                variant: 'info',
                duration: 2400,
              });
            },
          },
        });
      },
    });
  };
  $('#bulk-review-approve')?.addEventListener('click', () => applyBulkAction('approve'));
  $('#bulk-review-return')?.addEventListener('click', () => applyBulkAction('return'));
  $('#bulk-review-reject')?.addEventListener('click', () => applyBulkAction('reject'));
}

// Shared date-range dropdown (同樣的「全部 / 最近 7 天 / 最近 30 天 / 本月 /
// 上個月」選項 across 待審核、已審核、草稿區、已申請). Handles trigger click
// toggle, item selection + label update, default 'all' selection, and
// outside-click close. Callers pass the range var setter via `onChange`.
const DATE_RANGE_LABELS = {
  all: '全部', '7d': '最近 7 天', '30d': '最近 30 天', thisMonth: '本月', lastMonth: '上個月',
};
function initDateRangeFilter({ dropdownId, triggerId, labelId, onChange }) {
  const dd = document.getElementById(dropdownId);
  if (!dd) return;
  const trigger = document.getElementById(triggerId);
  const labelEl = document.getElementById(labelId);
  const defaultLabel = labelEl?.dataset.default || '日期';

  const applyLabel = (range) => {
    if (!labelEl) return;
    if (range === 'all') {
      labelEl.textContent = defaultLabel;
      trigger?.classList.remove('is-filtered');
    } else {
      labelEl.textContent = DATE_RANGE_LABELS[range] || defaultLabel;
      trigger?.classList.add('is-filtered');
    }
  };

  trigger?.addEventListener('click', (e) => {
    e.stopPropagation();
    toggleDropdownExclusive(dd);
  });
  dd.querySelectorAll('.dropdown-item').forEach(item => {
    item.addEventListener('click', () => {
      const range = item.getAttribute('data-range');
      dd.querySelectorAll('.dropdown-item').forEach(i => i.classList.remove('selected'));
      item.classList.add('selected');
      applyLabel(range);
      onChange(range);
      dd.classList.remove('open');
    });
  });
  document.addEventListener('click', (e) => {
    if (!dd.contains(e.target)) dd.classList.remove('open');
  });
  dd.querySelector('.dropdown-item[data-range="all"]')?.classList.add('selected');
  applyLabel('all');
}

function initReviewedFilters() {
  const statusDD = $('#reviewed-filter-dropdown');
  const statusTrigger = $('#reviewed-filter-trigger');
  const statusLabelEl = $('#reviewed-filter-label');
  const statusDefault = statusLabelEl?.dataset.default || '狀態';
  const statusLabels = { all: statusDefault, approved: '已核准', rejected: '已退回／已取消' };
  const dateDD = $('#reviewed-date-dropdown');

  if (statusDD) {
    statusTrigger?.addEventListener('click', (e) => {
      e.stopPropagation();
      toggleDropdownExclusive(statusDD);
    });
    $$('#reviewed-filter-dropdown .dropdown-item').forEach(item => {
      item.addEventListener('click', () => {
        const filter = item.getAttribute('data-filter');
        $$('#reviewed-filter-dropdown .dropdown-item').forEach(i => i.classList.remove('selected'));
        item.classList.add('selected');
        if (statusLabelEl) statusLabelEl.textContent = statusLabels[filter] || statusDefault;
        statusTrigger?.classList.toggle('is-filtered', filter !== 'all');
        currentReviewedStatus = filter;
        reviewedPage = 1;
        renderNotifications();
        iconsRefresh();
        statusDD.classList.remove('open');
      });
    });
  }

  // Date range: shared helper
  initDateRangeFilter({
    dropdownId: 'reviewed-date-dropdown',
    triggerId: 'reviewed-date-trigger',
    labelId: 'reviewed-date-label',
    onChange: (range) => {
      currentReviewedDateRange = range;
      reviewedPage = 1;
      renderNotifications();
      iconsRefresh();
    },
  });

  // Notified tab — date range filter (no status filter; all entries are 已知會)
  initDateRangeFilter({
    dropdownId: 'notified-date-dropdown',
    triggerId: 'notified-date-trigger',
    labelId: 'notified-date-label',
    onChange: (range) => {
      currentNotifiedDateRange = range;
      notifiedPage = 1;
      renderNotifications();
      iconsRefresh();
    },
  });

  document.addEventListener('click', () => statusDD?.classList.remove('open'));
  statusDD?.querySelector('.dropdown-item[data-filter="all"]')?.classList.add('selected');
}

function initDraftsDateFilter() {
  initDateRangeFilter({
    dropdownId: 'drafts-date-dropdown',
    triggerId: 'drafts-date-trigger',
    labelId: 'drafts-date-label',
    onChange: (range) => {
      currentDraftsDateRange = range;
      draftsPage = 1;
      bulkDraftsSelected.clear();
      renderDrafts();
      iconsRefresh();
    },
  });
}

function initPendingDateFilter() {
  initDateRangeFilter({
    dropdownId: 'pending-date-dropdown',
    triggerId: 'pending-date-trigger',
    labelId: 'pending-date-label',
    onChange: (range) => {
      currentPendingDateRange = range;
      pendingPage = 1;
      bulkReviewSelected.clear();
      renderNotifications();
      iconsRefresh();
    },
  });
}

// Form-type filter for pending list. Options are derived from distinct
// `title` values in PENDING so the dropdown only offers types that have
// data behind them.
function initPendingTypeFilter() {
  const dd = document.getElementById('pending-type-dropdown');
  const trigger = document.getElementById('pending-type-trigger');
  const labelEl = document.getElementById('pending-type-label');
  const menu = document.getElementById('pending-type-menu');
  if (!dd || !menu) return;
  const defaultLabel = labelEl?.dataset.default || '類型';

  const distinctTypes = [...new Set(PENDING.map(p => p.title))].sort();
  menu.innerHTML = `
    <div class="dropdown-item selected" data-type="all"><span>全部</span></div>
    ${distinctTypes.map(t => `<div class="dropdown-item" data-type="${t}"><span>${t}</span></div>`).join('')}
  `;

  trigger?.addEventListener('click', (e) => {
    e.stopPropagation();
    toggleDropdownExclusive(dd);
  });
  menu.querySelectorAll('.dropdown-item').forEach(item => {
    item.addEventListener('click', () => {
      const type = item.getAttribute('data-type');
      menu.querySelectorAll('.dropdown-item').forEach(i => i.classList.remove('selected'));
      item.classList.add('selected');
      if (labelEl) {
        labelEl.textContent = type === 'all' ? defaultLabel : type;
        trigger?.classList.toggle('is-filtered', type !== 'all');
      }
      currentPendingType = type;
      pendingPage = 1;
      bulkReviewSelected.clear();
      renderNotifications();
      iconsRefresh();
      dd.classList.remove('open');
    });
  });
  document.addEventListener('click', (e) => {
    if (!dd.contains(e.target)) dd.classList.remove('open');
  });
}

// ---------- Render: AI Review chat body ----------
function renderAIReview() {
  const body = $('#ai-review-body');
  body.innerHTML = `
    <div class="chat-message">
      <div class="chat-avatar-sm">🤖</div>
      <div class="chat-bubble">
        👋 早安 Tammy！你目前有 <b>4 個表單待處理</b>。<br>
        我已經先看過一輪，按時間排序為你準備建議。要我從最早的「請假申請單」開始嗎？
      </div>
    </div>
    <div class="chat-message user">
      <div class="chat-avatar-sm user">T</div>
      <div class="chat-bubble">好，先看王小明那筆請假。</div>
    </div>
    <div class="chat-message">
      <div class="chat-avatar-sm">🤖</div>
      <div class="chat-bubble">
        這是王小明提出的特休申請：
        <div class="review-card">
          <div class="review-card-title">請假申請單 · 王小明</div>
          <div class="review-card-row"><span class="label">假別</span><span>特休 · 全天</span></div>
          <div class="review-card-row"><span class="label">日期</span><span>2026/4/20（一）</span></div>
          <div class="review-card-row"><span class="label">事由</span><span>家人返國陪伴</span></div>
          <div class="review-card-row"><span class="label">職務代理</span><span>Ella Wang（已同意）</span></div>
          <div class="review-card-row"><span class="label">剩餘特休</span><span>9 天（含本申請）</span></div>
          <div class="review-card-divider"></div>
          <div class="review-card-actions">
            <button class="btn btn-outline btn-sm" id="review-view-form-btn">查看表單</button>
          </div>
        </div>
        <div style="margin-top:12px">
          <b>我的建議：</b>✅ 建議核准 — 特休是員工權利、代理人已接手、時程單純。
        </div>
        <div class="review-action-row">
          <button class="btn btn-destructive btn-sm">
            <i data-lucide="x" class="icon"></i> 不通過
          </button>
          <button class="btn btn-outline btn-sm">
            <i data-lucide="corner-up-left" class="icon"></i> 退回補件
          </button>
          <button class="btn btn-primary btn-sm">
            <i data-lucide="check" class="icon"></i> 核准
          </button>
        </div>
      </div>
    </div>
  `;

  $('#review-view-form-btn')?.addEventListener('click', () => {
    openFormModal('請假申請單', 'review', {
      who: '王小明（Engineering）',
      desc: '家人從國外回來探親，希望能陪伴一天。',
      time: '4/14',
      formNo: 'NX-2026-0420',
    });
  });
}

// ---------- Render: Form categories ----------
let formListSearchQuery = '';

function renderFormCategories() {
  const root = $('#form-categories');
  root.innerHTML = '';
  const q = formListSearchQuery.trim().toLowerCase();
  const matches = (it) => !q ||
    (it.title || '').toLowerCase().includes(q) ||
    (it.desc || '').toLowerCase().includes(q);

  const filteredCats = FORM_CATEGORIES
    .map(cat => ({ ...cat, items: cat.items.filter(matches) }))
    .filter(cat => cat.items.length > 0);

  if (filteredCats.length === 0) {
    root.innerHTML = `
      <div class="empty-state" style="grid-column:1/-1">
        <i data-lucide="search-x" class="icon"></i>
        <div class="h4">找不到符合的表單</div>
        <div class="p-small muted">請嘗試其他關鍵字</div>
      </div>
    `;
    iconsRefresh();
    return;
  }

  filteredCats.forEach(cat => {
    const block = el('div', 'form-column');
    block.innerHTML = `
      <div class="form-column-head">
        <div class="p-medium">${cat.title}</div>
      </div>
      <div class="form-list">
        ${cat.items.map(it => `
          <div class="form-item" data-title="${it.title}">
            <div class="form-item-main">
              <span class="form-item-emoji">${it.emoji}</span>
              <div class="form-item-text">
                <div class="form-item-title">${it.title}</div>
                <div class="form-item-desc">${it.desc}</div>
              </div>
            </div>
            <i data-lucide="chevron-right" class="chevron"></i>
          </div>
        `).join('')}
      </div>
    `;
    block.querySelectorAll('.form-item').forEach(row => {
      row.addEventListener('click', () => {
        openFormModal(row.getAttribute('data-title'));
      });
    });
    root.appendChild(block);
  });
}

// ---------- Render: Drafts list ----------
let currentDraftsDateRange = 'all';
let draftsPage = 1;

// Bulk delete: tracks which DRAFTS the user has checked
const bulkDraftsSelected = new Set();

function renderDrafts() {
  const root = $('#form-drafts-list');
  const countEl = $('#drafts-count');
  if (!root) return;
  if (countEl) countEl.textContent = DRAFTS.length;

  const items = DRAFTS.filter(d => isRelativeInDateRange(d.updated, currentDraftsDateRange));
  const totalPages = Math.max(1, Math.ceil(items.length / LIST_PAGE_SIZE));
  if (draftsPage > totalPages) draftsPage = totalPages;
  const pageItems = items.slice((draftsPage - 1) * LIST_PAGE_SIZE, draftsPage * LIST_PAGE_SIZE);

  if (items.length === 0) {
    root.innerHTML = `
      <tr><td colspan="5"><div class="empty-state">
        <i data-lucide="file-edit" class="icon"></i>
        <div class="h4">${DRAFTS.length === 0 ? '目前沒有草稿' : '此條件下沒有草稿'}</div>
        <div class="p-small muted">${DRAFTS.length === 0 ? '在表單中點選「儲存草稿」後會顯示在這裡。' : '請嘗試其他日期範圍。'}</div>
      </div></td></tr>
    `;
    bulkDraftsSelected.clear();
    updateBulkDraftsUI([]);
    renderPagination({ hostId: 'drafts-pagination', totalItems: 0, page: 1, pageSize: LIST_PAGE_SIZE, onPageChange: () => {} });
    iconsRefresh();
    return;
  }

  root.innerHTML = pageItems.map(d => `
    <tr class="notif-row-tr" data-title="${d.title}">
      <td class="emp-check-td">
        <span class="notif-item-select bulk-review-check" role="checkbox" aria-checked="false" tabindex="0" aria-label="選取此草稿"></span>
      </td>
      <td class="notif-status-cell"><span class="badge badge-secondary">草稿</span></td>
      <td class="notif-content-cell">
        <div class="form-draft-main">
          <span class="form-draft-emoji">${d.emoji}</span>
          <div class="form-draft-text">
            <div class="form-draft-title-row">
              <span class="form-draft-title">${d.title}</span>
              <span class="badge badge-secondary notif-badge-inline">草稿</span>
            </div>
            <div class="form-draft-summary">${d.summary}</div>
            <div class="form-draft-time-inline">${fmtDateLong(d.updated)}</div>
          </div>
        </div>
      </td>
      <td class="notif-time-cell p-small muted">${fmtDateLong(d.updated)}</td>
      <td class="notif-chevron-cell"><i data-lucide="chevron-right" class="icon"></i></td>
    </tr>
  `).join('');

  // After the new <tr>-based draft rendering, attach checkbox + row click
  // handlers. Selector matches the TR class used in renderDraftsRowHtml.
  root.querySelectorAll('.notif-row-tr').forEach((item, i) => {
    const draft = pageItems[i];
    const check = item.querySelector('.notif-item-select');
    if (bulkDraftsSelected.has(draft)) {
      check?.setAttribute('aria-checked', 'true');
      item.classList.add('bulk-selected');
    }
    check?.addEventListener('click', (e) => {
      e.stopPropagation();
      toggleBulkDraftItem(draft, item);
    });
    check?.addEventListener('keydown', (e) => {
      if (e.key === ' ' || e.key === 'Enter') {
        e.preventDefault();
        e.stopPropagation();
        toggleBulkDraftItem(draft, item);
      }
    });
    // Clicking the row opens the draft; clicking the checkbox is stopped above
    item.addEventListener('click', () => {
      openFormModal(item.getAttribute('data-title'), 'draft');
    });
  });

  updateBulkDraftsUI(pageItems);
  renderPagination({
    hostId: 'drafts-pagination',
    totalItems: items.length,
    page: draftsPage,
    pageSize: LIST_PAGE_SIZE,
    onPageChange: (p) => { draftsPage = p; renderDrafts(); },
  });
  iconsRefresh();  // convert <i data-lucide> markers (chevrons) to real SVGs
}

function toggleBulkDraftItem(draft, itemEl) {
  if (bulkDraftsSelected.has(draft)) {
    bulkDraftsSelected.delete(draft);
    itemEl.classList.remove('bulk-selected');
    itemEl.querySelector('.notif-item-select')?.setAttribute('aria-checked', 'false');
  } else {
    bulkDraftsSelected.add(draft);
    itemEl.classList.add('bulk-selected');
    itemEl.querySelector('.notif-item-select')?.setAttribute('aria-checked', 'true');
  }
  const items = DRAFTS.filter(d => isRelativeInDateRange(d.updated, currentDraftsDateRange));
  const pageItems = items.slice((draftsPage - 1) * LIST_PAGE_SIZE, draftsPage * LIST_PAGE_SIZE);
  updateBulkDraftsUI(pageItems);
}

function updateBulkDraftsUI(pageItems) {
  const count = bulkDraftsSelected.size;
  const actionbar = $('#bulk-drafts-actionbar');
  const countEl = $('#bulk-drafts-count');
  const summary = $('#bulk-drafts-summary');
  const selectAll = $('#bulk-drafts-selectall-check');
  const selectAllLabel = $('#bulk-drafts-selectall-label');

  if (countEl) countEl.textContent = count;
  if (actionbar) actionbar.hidden = count === 0;

  const pageSelectedCount = pageItems.filter(d => bulkDraftsSelected.has(d)).length;
  if (selectAll) {
    if (pageSelectedCount === 0) selectAll.setAttribute('aria-checked', 'false');
    else if (pageSelectedCount === pageItems.length) selectAll.setAttribute('aria-checked', 'true');
    else selectAll.setAttribute('aria-checked', 'mixed');
  }
  if (selectAllLabel) selectAllLabel.textContent = count > 0 ? `已選 ${count} 件` : '全選';
  if (summary) summary.textContent = pageItems.length > 0 ? `共 ${pageItems.length} 件` : '';
}

function initBulkDrafts() {
  const selectAll = $('#bulk-drafts-selectall-check');
  const selectAllLabel = $('#bulk-drafts-selectall-label')?.closest('.bulk-review-selectall');
  const toggleSelectAll = () => {
    const items = DRAFTS.filter(d => isRelativeInDateRange(d.updated, currentDraftsDateRange));
    const pageItems = items.slice((draftsPage - 1) * LIST_PAGE_SIZE, draftsPage * LIST_PAGE_SIZE);
    const allSelected = pageItems.length > 0 && pageItems.every(d => bulkDraftsSelected.has(d));
    pageItems.forEach(d => {
      if (allSelected) bulkDraftsSelected.delete(d);
      else bulkDraftsSelected.add(d);
    });
    renderDrafts();
  };
  selectAll?.addEventListener('click', toggleSelectAll);
  selectAllLabel?.addEventListener('click', (e) => {
    if (e.target === selectAll) return;
    toggleSelectAll();
  });
  selectAll?.addEventListener('keydown', (e) => {
    if (e.key === ' ' || e.key === 'Enter') {
      e.preventDefault();
      toggleSelectAll();
    }
  });

  $('#bulk-drafts-cancel')?.addEventListener('click', () => {
    bulkDraftsSelected.clear();
    renderDrafts();
  });

  $('#bulk-drafts-delete')?.addEventListener('click', () => {
    const count = bulkDraftsSelected.size;
    if (count === 0) return;
    confirmDialog({
      title: `確定要刪除 ${count} 件草稿？`,
      desc: '送出後 6 秒內可點「還原」撤回。',
      confirmText: '確定刪除',
      onConfirm: () => {
        const snapshot = [...bulkDraftsSelected]
          .map(d => ({ item: d, index: DRAFTS.indexOf(d) }))
          .filter(x => x.index >= 0)
          .sort((a, b) => a.index - b.index);

        snapshot.slice().reverse().forEach(({ index }) => DRAFTS.splice(index, 1));
        bulkDraftsSelected.clear();
        renderDrafts();

        showToast({
          title: `已刪除 ${count} 件草稿`,
          desc: '批次刪除完成',
          variant: 'info',
          action: {
            label: '還原',
            onClick: () => {
              snapshot.forEach(({ item, index }) => {
                const insertAt = Math.min(index, DRAFTS.length);
                DRAFTS.splice(insertAt, 0, item);
              });
              renderDrafts();
              showToast({
                title: '已還原',
                desc: `${count} 件草稿已恢復`,
                variant: 'info',
                duration: 2400,
              });
            },
          },
        });
      },
    });
  });
}

// ---------- Render: Applied-forms list ----------
let currentAppliedStatus = 'all';
let currentAppliedDateRange = 'all';
let appliedPage = 1;

function parseApplyDate(str) {
  const [y, m, d] = str.split('/').map(Number);
  return new Date(y, m - 1, d);
}

function isInDateRange(dateStr, range) {
  if (range === 'all') return true;
  const d = parseApplyDate(dateStr);
  const now = new Date();
  now.setHours(23, 59, 59, 999);
  switch (range) {
    case '7d': {
      const since = new Date(now);
      since.setDate(since.getDate() - 7);
      return d >= since && d <= now;
    }
    case '30d': {
      const since = new Date(now);
      since.setDate(since.getDate() - 30);
      return d >= since && d <= now;
    }
    case 'thisMonth':
      return d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth();
    case 'lastMonth': {
      const lm = new Date(now.getFullYear(), now.getMonth() - 1, 1);
      return d.getFullYear() === lm.getFullYear() && d.getMonth() === lm.getMonth();
    }
    default:
      return true;
  }
}

function filterAppliedForms(status = currentAppliedStatus, dateRange = currentAppliedDateRange) {
  let items = FORM_HISTORY;
  if (status === 'processing')      items = items.filter(r => r.status === 'warning' || r.status === 'info');
  else if (status === 'approved')   items = items.filter(r => r.status === 'success');
  else if (status === 'rejected')   items = items.filter(r => r.status === 'destructive' || r.status === 'secondary');
  if (dateRange !== 'all')          items = items.filter(r => isInDateRange(r.date, dateRange));
  return items;
}

function updateAppliedFilterCounts() {
  // Counts reflect the *current date range*, so status numbers stay consistent with what user sees
  const counts = {
    all:        filterAppliedForms('all',        currentAppliedDateRange).length,
    processing: filterAppliedForms('processing', currentAppliedDateRange).length,
    approved:   filterAppliedForms('approved',   currentAppliedDateRange).length,
    rejected:   filterAppliedForms('rejected',   currentAppliedDateRange).length,
  };
  Object.entries(counts).forEach(([key, val]) => {
    const el = document.querySelector(`#applied-filter-dropdown [data-count="${key}"]`);
    if (el) el.textContent = val;
  });
}

function renderAppliedForms(status = currentAppliedStatus) {
  currentAppliedStatus = status;
  const root = $('#form-applied-list');
  const countEl = $('#applied-count');
  if (!root) return;

  const typeEmoji = {
    '人事考勤': '🗓️',
    '財務費用': '💰',
    '行政庶務': '🧰',
    '通用管理': '📝',
  };
  const statusBadge = {
    success:     'badge-success',
    warning:     'badge-warning',
    info:        'badge-info',
    destructive: 'badge-destructive',
    secondary:   'badge-secondary',
  };

  updateAppliedFilterCounts();
  if (countEl) countEl.textContent = FORM_HISTORY.length;

  const items = filterAppliedForms(status, currentAppliedDateRange);
  const totalPages = Math.max(1, Math.ceil(items.length / LIST_PAGE_SIZE));
  if (appliedPage > totalPages) appliedPage = totalPages;
  const pageItems = items.slice((appliedPage - 1) * LIST_PAGE_SIZE, appliedPage * LIST_PAGE_SIZE);

  if (items.length === 0) {
    root.innerHTML = `
      <tr><td colspan="4"><div class="empty-state">
        <i data-lucide="file-check-2" class="icon"></i>
        <div class="h4">此分類目前沒有申請</div>
      </div></td></tr>
    `;
    renderPagination({ hostId: 'form-applied-pagination', totalItems: 0, page: 1, pageSize: LIST_PAGE_SIZE, onPageChange: () => {} });
    return;
  }

  root.innerHTML = pageItems.map(row => {
    const emoji = typeEmoji[row.type] || '📄';
    const badgeCls = statusBadge[row.status] || 'badge-secondary';
    const parts = row.title.split(/\s*[—–-]\s*/);
    const mainTitle = parts[0] || row.title;
    const desc = parts.slice(1).join(' — ');
    const hasApprover = row.approver && row.approver !== '—';
    const metaBits = [
      hasApprover ? `核決者：${row.approver}` : '',
      desc || '',
    ].filter(Boolean).join(' · ');
    return `
      <tr class="notif-row-tr" data-title="${row.title}">
        <td class="notif-status-cell"><span class="badge ${badgeCls}">${row.statusText}</span></td>
        <td class="notif-content-cell">
          <div class="form-draft-main">
            <span class="form-draft-emoji">${emoji}</span>
            <div class="form-draft-text">
              <div class="form-draft-title-row">
                <span class="form-draft-title">${mainTitle}</span>
                <span class="badge ${badgeCls} notif-badge-inline">${row.statusText}</span>
              </div>
              <div class="form-draft-meta">${metaBits}</div>
              <div class="form-draft-time-inline">${fmtDateLong(row.date)}</div>
            </div>
          </div>
        </td>
        <td class="notif-time-cell p-small muted">${fmtDateLong(row.date)}</td>
        <td class="notif-chevron-cell"><i data-lucide="chevron-right" class="icon"></i></td>
      </tr>
    `;
  }).join('');

  // Row click → open the form in view-only mode (matches the other list views)
  root.querySelectorAll('.notif-row-tr').forEach((tr) => {
    tr.addEventListener('click', () => {
      const title = tr.getAttribute('data-title');
      if (title) openFormModal(title, 'view');
    });
  });

  renderPagination({
    hostId: 'form-applied-pagination',
    totalItems: items.length,
    page: appliedPage,
    pageSize: LIST_PAGE_SIZE,
    onPageChange: (p) => { appliedPage = p; renderAppliedForms(currentAppliedStatus); },
  });
  iconsRefresh();  // render chevron-right SVGs
}

function initAppliedFilters() {
  // Status dropdown
  const statusDD = $('#applied-filter-dropdown');
  const statusTrigger = $('#applied-filter-trigger');
  const statusLabelEl = $('#applied-filter-label');
  const statusDefault = statusLabelEl?.dataset.default || '狀態';
  const statusLabels = { all: statusDefault, processing: '審核中', approved: '已核准', rejected: '已退回／已取消' };

  if (statusDD) {
    statusTrigger?.addEventListener('click', (e) => {
      e.stopPropagation();
      toggleDropdownExclusive(statusDD);
    });
    $$('#applied-filter-dropdown .dropdown-item').forEach(item => {
      item.addEventListener('click', () => {
        const filter = item.getAttribute('data-filter');
        $$('#applied-filter-dropdown .dropdown-item').forEach(i => i.classList.remove('selected'));
        item.classList.add('selected');
        if (statusLabelEl) statusLabelEl.textContent = statusLabels[filter] || statusDefault;
        statusTrigger?.classList.toggle('is-filtered', filter !== 'all');
        appliedPage = 1;
        renderAppliedForms(filter);
        iconsRefresh();
        statusDD.classList.remove('open');
      });
    });
    statusDD.querySelector('.dropdown-item[data-filter="all"]')?.classList.add('selected');
  }

  // Date range: shared helper
  initDateRangeFilter({
    dropdownId: 'applied-date-dropdown',
    triggerId: 'applied-date-trigger',
    labelId: 'applied-date-label',
    onChange: (range) => {
      currentAppliedDateRange = range;
      appliedPage = 1;
      renderAppliedForms(currentAppliedStatus);
      iconsRefresh();
    },
  });

  document.addEventListener('click', (e) => {
    if (statusDD && !statusDD.contains(e.target)) statusDD.classList.remove('open');
  });
}

// ---------- Render: Form AI chat body ----------
function renderFormChat() {
  $('#form-chat-body').innerHTML = `
    <div class="chat-message">
      <div class="chat-avatar-sm">📋</div>
      <div class="chat-bubble">
        哈囉！我是 <b>AI 表單助理</b>。告訴我你想處理什麼事情，我能推薦最適合的表單並協助你填寫。
      </div>
    </div>
    <div class="chat-message user">
      <div class="chat-avatar-sm user">T</div>
      <div class="chat-bubble">我下週要去東京出差 3 天，想先做準備。</div>
    </div>
    <div class="chat-message">
      <div class="chat-avatar-sm">📋</div>
      <div class="chat-bubble">
        建議你使用「<b>出差申請單</b>」，並搭配「差旅費核銷單」於返國後處理。
        <div class="review-card">
          <div class="review-card-title">出差申請單 · 草稿</div>
          <div class="review-card-row"><span class="label">出差地點</span><span>日本 · 東京</span></div>
          <div class="review-card-row"><span class="label">日期</span><span>2026/5/5（二）– 5/7（四）· 3 天</span></div>
          <div class="review-card-row"><span class="label">預估費用</span><span>NT$42,000（含機票、住宿、餐費）</span></div>
          <div class="review-card-row"><span class="label">目的</span><span>AI 研討會、客戶拜訪</span></div>
          <div class="review-action-row">
            <button class="btn btn-primary btn-sm" id="form-chat-edit-btn">編輯表單</button>
          </div>
        </div>
        需要我一併幫你準備出差交接單、或產生行程草案嗎？
      </div>
    </div>
  `;

  $('#form-chat-edit-btn')?.addEventListener('click', () => {
    openFormModal('出差申請單');
  });
}

// ---------- Render: Assistants grid ----------
let assistantsFilterTag = 'all';
let assistantsSearchQuery = '';

function renderAssistantsGrid(filter = assistantsFilterTag) {
  assistantsFilterTag = filter;
  const grid = $('#assistants-grid');
  if (!grid) return;
  grid.innerHTML = '';
  const q = assistantsSearchQuery.trim().toLowerCase();
  let items = filter === 'all' ? ASSISTANTS : ASSISTANTS.filter(a => a.tag === filter);
  if (q) {
    items = items.filter(a =>
      a.title.toLowerCase().includes(q) ||
      (a.desc || '').toLowerCase().includes(q) ||
      (a.tag || '').toLowerCase().includes(q)
    );
  }
  items = items.slice(0, 9);

  if (items.length === 0) {
    grid.innerHTML = `
      <div class="empty-state assistants-empty-state">
        <i data-lucide="search-x" class="icon"></i>
        <div class="h4">找不到符合的助理</div>
        <div class="p-small muted">請嘗試其他關鍵字或切換分類</div>
      </div>
    `;
    return;
  }

  items.forEach(a => {
    const card = el('div', 'assistant-grid-card');
    card.innerHTML = `
      <div class="assistant-emoji">${a.emoji}</div>
      <div class="assistant-grid-content">
        <div class="assistant-grid-title">${a.title}</div>
        <div class="assistant-grid-desc">${a.desc}</div>
      </div>
    `;
    card.addEventListener('click', () => switchView('chat', { assistant: a }));
    grid.appendChild(card);
  });
}

// ---------- Render: History tables ----------
let historySearchQuery = '';

const HISTORY_MAX_DISPLAY = 30;

function renderHistoryChat() {
  const tbody = $('#history-chat-body');
  if (!tbody) return;
  const q = historySearchQuery.trim().toLowerCase();
  const latest = CHAT_HISTORY.slice(0, HISTORY_MAX_DISPLAY); // latest 30 (already newest-first)
  const items = latest
    .map((row, i) => ({ ...row, _idx: i }))
    .filter(row => !q ||
      row.title.toLowerCase().includes(q) ||
      (row.assist || '').toLowerCase().includes(q));

  if (items.length === 0) {
    tbody.innerHTML = `<tr><td colspan="3" style="text-align:center;color:var(--muted-foreground);padding:var(--space-xl)">找不到符合的對話</td></tr>`;
    return;
  }

  const reachedCap = !q && CHAT_HISTORY.length > HISTORY_MAX_DISPLAY;
  const formatHistoryTime = (raw) => fmtDateLong(raw || '');

  tbody.innerHTML = items.map(row => `
    <tr class="history-row" data-row-idx="${row._idx}">
      <td>
        <div class="table-title">${row.title}</div>
      </td>
      <td class="history-time-cell">${formatHistoryTime(row.time)}</td>
      <td class="table-actions">
        <button class="icon-button history-chat-delete" aria-label="刪除"><i data-lucide="trash-2" class="icon"></i></button>
        <i data-lucide="chevron-right" class="icon history-row-chevron"></i>
      </td>
    </tr>
  `).join('') + (reachedCap
    ? `<tr class="history-cap-row"><td colspan="3" class="history-cap-cell">僅顯示最新 ${HISTORY_MAX_DISPLAY} 筆</td></tr>`
    : '');

  // Row click → open AI chat inner page
  tbody.querySelectorAll('.history-row').forEach(tr => {
    tr.addEventListener('click', (e) => {
      if (e.target.closest('.history-chat-delete')) return;
      const idx = parseInt(tr.dataset.rowIdx ?? '-1', 10);
      closeHistoryListModal();
      switchView('chat', { assistant: ASSISTANTS[0], history: CHAT_HISTORY[idx] });
    });
  });

  tbody.querySelectorAll('.history-chat-delete').forEach(btn => {
    btn.addEventListener('click', (e) => {
      e.stopPropagation();
      const tr = btn.closest('tr');
      const idx = parseInt(tr?.dataset.rowIdx ?? '-1', 10);
      const title = CHAT_HISTORY[idx]?.title || '此對話';
      confirmDialog({
        title: '確定要刪除此對話？',
        desc: `「${title}」將被移除，此動作無法復原。`,
        confirmText: '刪除',
        onConfirm: () => tr?.remove(),
      });
    });
  });
}

function openHistoryModal(idx) {
  const modal = $('#history-modal');
  const row = CHAT_HISTORY[idx];
  if (!modal || !row) return;
  $('#history-modal-title').textContent = row.title;
  $('#history-modal-sub').textContent = `${row.assist} · 最後更新 ${fmtDateLong(row.time)} · 共 ${row.count} 則訊息`;

  // Render a simple mock transcript
  const body = $('#history-modal-body');
  if (body) {
    body.innerHTML = `
      <div class="chat-body" style="padding:0">
        <div class="chat-message">
          <div class="chat-avatar-sm">🤖</div>
          <div class="chat-bubble">${row.title}，我可以幫你從這裡開始。</div>
        </div>
        <div class="chat-message user">
          <div class="chat-avatar-sm user">T</div>
          <div class="chat-bubble">好的，麻煩幫我整理。</div>
        </div>
        <div class="chat-message">
          <div class="chat-avatar-sm">🤖</div>
          <div class="chat-bubble">已為你整理好重點，若需要完整紀錄請開啟對應助理的對話。</div>
        </div>
      </div>
    `;
  }

  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  iconsRefresh();
}

function initHistoryModal() {
  const modal = $('#history-modal');
  if (!modal) return;
  const close = () => {
    modal.classList.remove('open');
    modal.setAttribute('aria-hidden', 'true');
  };
  $('#history-modal-close')?.addEventListener('click', close);
  modal.addEventListener('click', (e) => { if (e.target === modal) close(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && modal.classList.contains('open')) close();
  });
}

function openHistoryListModal() {
  const modal = $('#history-list-modal');
  if (!modal) return;
  historySearchQuery = '';
  const searchInput = $('#history-search-input');
  if (searchInput) searchInput.value = '';
  renderHistoryChat();
  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  iconsRefresh();
}

function closeHistoryListModal() {
  const modal = $('#history-list-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
}

function initHistoryListModal() {
  const modal = $('#history-list-modal');
  if (!modal) return;
  $('#history-list-modal-close')?.addEventListener('click', closeHistoryListModal);
  modal.addEventListener('click', (e) => { if (e.target === modal) closeHistoryListModal(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && modal.classList.contains('open')) closeHistoryListModal();
  });
  $('#history-search-input')?.addEventListener('input', (e) => {
    historySearchQuery = e.target.value || '';
    renderHistoryChat();
    iconsRefresh();
  });
}

// ---------- Tabs ----------
function initTabs() {
  $$('[data-tabs]').forEach(group => {
    const tabs = $$('.tab, .notif-tab', group);
    const panelIds = tabs.map(t => t.getAttribute('data-tab')).filter(Boolean);

    // ARIA roles + keyboard navigation
    group.setAttribute('role', 'tablist');
    tabs.forEach((tab, i) => {
      const panelId = tab.getAttribute('data-tab');
      const isActive = tab.classList.contains('active');
      tab.setAttribute('role', 'tab');
      tab.setAttribute('aria-selected', isActive ? 'true' : 'false');
      tab.setAttribute('tabindex', isActive ? '0' : '-1');
      if (panelId) {
        const panel = document.getElementById('tab-' + panelId);
        if (panel) {
          const panelDomId = panel.id;
          const tabDomId = tab.id || `tab-ctrl-${panelDomId}`;
          tab.id = tabDomId;
          tab.setAttribute('aria-controls', panelDomId);
          panel.setAttribute('role', 'tabpanel');
          panel.setAttribute('aria-labelledby', tabDomId);
          panel.setAttribute('tabindex', '0');
        }
      }

      tab.addEventListener('keydown', (e) => {
        let target = null;
        if (e.key === 'ArrowRight' || e.key === 'ArrowDown') target = tabs[(i + 1) % tabs.length];
        else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') target = tabs[(i - 1 + tabs.length) % tabs.length];
        else if (e.key === 'Home') target = tabs[0];
        else if (e.key === 'End') target = tabs[tabs.length - 1];
        if (target) {
          e.preventDefault();
          target.click();
          target.focus();
        }
      });
    });

    tabs.forEach(tab => {
      tab.addEventListener('click', () => {
        tabs.forEach(t => {
          t.classList.remove('active');
          t.setAttribute('aria-selected', 'false');
          t.setAttribute('tabindex', '-1');
        });
        tab.classList.add('active');
        tab.setAttribute('aria-selected', 'true');
        tab.setAttribute('tabindex', '0');
        const groupName = group.getAttribute('data-tabs');
        const panelId = tab.getAttribute('data-tab');

        if (panelId) {
          panelIds.forEach(id => {
            const p = document.getElementById('tab-' + id);
            if (p) p.classList.toggle('active', id === panelId);
          });
        }

        // Assistants filter
        if (groupName === 'assist-filter') {
          renderAssistantsGrid(panelId);
          iconsRefresh();
        }
      });
    });
  });
}

// ---------- Sidebar nav ----------
// Wire ARIA roles + keyboard navigation onto every `.dropdown`. Filter
// dropdowns (items with [data-filter]/[data-range]/[data-product]/...)
// get `role="listbox"`; plain menus (user/avatar) get `role="menu"`.
// Trigger gets `aria-haspopup` + `aria-expanded` synced to the `.open` class.
function initDropdownsA11y() {
  document.querySelectorAll('.dropdown').forEach(dd => {
    const trigger = dd.querySelector(
      ':scope > button, :scope > .avatar-trigger, :scope > [id$="-trigger"]'
    );
    const menu = dd.querySelector(':scope > .dropdown-menu');
    if (!trigger || !menu) return;

    const isListbox = !!menu.querySelector(
      '[data-filter], [data-range], [data-product], [data-category]'
    );
    const menuRole = isListbox ? 'listbox' : 'menu';
    const itemRole = isListbox ? 'option' : 'menuitem';

    // Ensure trigger is tab-reachable and announces popup behavior
    if (!trigger.hasAttribute('tabindex') && trigger.tagName !== 'BUTTON') {
      trigger.setAttribute('tabindex', '0');
      trigger.setAttribute('role', 'button');
    }
    const triggerId = trigger.id || `${dd.id || 'dropdown'}-trigger`;
    trigger.id = triggerId;
    trigger.setAttribute('aria-haspopup', menuRole);
    trigger.setAttribute('aria-expanded', dd.classList.contains('open') ? 'true' : 'false');
    trigger.setAttribute('aria-controls', menu.id || (menu.id = `${triggerId}-menu`));

    menu.setAttribute('role', menuRole);
    menu.setAttribute('aria-labelledby', triggerId);

    const items = [...menu.querySelectorAll('.dropdown-item')];
    items.forEach(item => {
      item.setAttribute('role', itemRole);
      if (isListbox) {
        item.setAttribute('aria-selected', item.classList.contains('selected') ? 'true' : 'false');
      }
      if (!item.hasAttribute('tabindex')) item.setAttribute('tabindex', '-1');
    });

    // Sync aria-expanded + aria-selected whenever state changes
    const mo = new MutationObserver(() => {
      const open = dd.classList.contains('open');
      trigger.setAttribute('aria-expanded', open ? 'true' : 'false');
      if (isListbox) {
        items.forEach(item => {
          item.setAttribute('aria-selected', item.classList.contains('selected') ? 'true' : 'false');
        });
      }
    });
    mo.observe(dd, { attributes: true, attributeFilter: ['class'] });
    items.forEach(item => {
      mo.observe(item, { attributes: true, attributeFilter: ['class'] });
    });

    // Keyboard navigation on the trigger
    trigger.addEventListener('keydown', (e) => {
      if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
        if (!dd.classList.contains('open')) {
          e.preventDefault();
          trigger.click();
        }
        const focusTarget = menu.querySelector('.dropdown-item.selected') || items[0];
        focusTarget?.focus();
      } else if (e.key === 'Escape' && dd.classList.contains('open')) {
        dd.classList.remove('open');
      }
    });

    // Keyboard navigation within the menu
    items.forEach((item, i) => {
      item.addEventListener('keydown', (e) => {
        let target = null;
        if (e.key === 'ArrowDown') target = items[(i + 1) % items.length];
        else if (e.key === 'ArrowUp') target = items[(i - 1 + items.length) % items.length];
        else if (e.key === 'Home') target = items[0];
        else if (e.key === 'End') target = items[items.length - 1];
        else if (e.key === 'Escape') { dd.classList.remove('open'); trigger.focus(); return; }
        else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); item.click(); return; }
        if (target) { e.preventDefault(); target.focus(); }
      });
    });
  });
}

function initSidebar() {
  $$('.sidebar-item[data-view], .sidebar-item[data-modal]').forEach(item => {
    // Derive tooltip text from the label span (ignore badge spans)
    if (!item.getAttribute('aria-label')) {
      const labelSpan = [...item.querySelectorAll('span')].find(s => !s.classList.contains('sidebar-badge'));
      if (labelSpan) item.setAttribute('aria-label', labelSpan.textContent.trim());
    }
    item.addEventListener('click', () => {
      const view = item.getAttribute('data-view');
      const modalKey = item.getAttribute('data-modal');
      if (modalKey === 'history') {
        openHistoryListModal();
        return;
      }
      if (view) switchView(view);
    });
  });

  $$('[data-view-link]').forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      switchView(link.getAttribute('data-view-link'));
    });
  });

  const newChatBtn = $('#new-chat-btn');
  if (newChatBtn) newChatBtn.addEventListener('click', () => {
    switchView('chat', { assistant: ASSISTANTS[0] });
  });

  // Sidebar toggle — three entry points share the same collapse/expand logic:
  //   * #sidebar-toggle-btn (header hamburger, mobile only)
  //   * #sidebar-collapse-btn (in-sidebar row, desktop expanded only)
  //   * #sidebar-logo (desktop collapsed → expand; desktop expanded → nav home)
  const toggleBtn = $('#sidebar-toggle-btn');
  const collapseBtn = $('#sidebar-collapse-btn');
  const sidebarLogo = $('#sidebar-logo');
  const isMobile = () => window.matchMedia('(max-width: 720px)').matches;
  const app = $('.app');

  function syncToggleLabels() {
    if (!app) return;
    if (toggleBtn) {
      if (isMobile()) {
        const open = app.classList.contains('sidebar-open');
        toggleBtn.setAttribute('aria-label', open ? '關閉導覽列' : '展開導覽列');
      } else {
        toggleBtn.setAttribute('aria-label', '切換導覽列');
      }
    }
    if (collapseBtn) {
      collapseBtn.setAttribute('aria-label',
        app.classList.contains('sidebar-collapsed') ? '展開導覽列' : '收起導覽列');
    }
    if (sidebarLogo) {
      const collapsed = app.classList.contains('sidebar-collapsed');
      const label = collapsed && !isMobile() ? '展開導覽列' : '回主頁';
      sidebarLogo.setAttribute('aria-label', label);
      // Keep native title for the expanded state (hover delay feels right
      // for a nav-home affordance); clear it when collapsed because the
      // CSS tooltip takes over.
      if (collapsed && !isMobile()) sidebarLogo.removeAttribute('title');
      else sidebarLogo.setAttribute('title', '回主頁');
    }
  }

  function toggleCollapse() {
    if (!app) return;
    app.classList.toggle('sidebar-collapsed');
    const collapsed = app.classList.contains('sidebar-collapsed');
    // Keep the old header-toggle icons in sync too (for the mobile case
    // where the header button remains relevant)
    const closeIcon = $('#sidebar-icon-close');
    const openIcon = $('#sidebar-icon-open');
    if (closeIcon) closeIcon.style.display = collapsed ? 'none' : '';
    if (openIcon) openIcon.style.display = collapsed ? '' : 'none';
    syncToggleLabels();
  }

  syncToggleLabels();

  if (toggleBtn) {
    toggleBtn.addEventListener('click', () => {
      if (!app) return;
      if (isMobile()) {
        const opening = !app.classList.contains('sidebar-open');
        if (opening) closeAllAIPanels();
        app.classList.toggle('sidebar-open');
        syncToggleLabels();
      } else {
        toggleCollapse();
      }
    });
    // Close drawer when selecting a sidebar item on mobile (covers both
    // the main app sidebar and the workspace sidebar, which share the
    // same .sidebar-open drawer mechanism).
    $$('.sidebar-item[data-view], .sidebar-item[data-modal], .workspace-nav-item, .workspace-back').forEach(item => {
      item.addEventListener('click', () => {
        if (isMobile()) $('.app')?.classList.remove('sidebar-open');
      });
    });
    // Close drawer when clicking the backdrop (the ::after overlay)
    document.addEventListener('click', (e) => {
      if (!isMobile()) return;
      if (!app?.classList.contains('sidebar-open')) return;
      // Whichever drawer is currently visible counts as "inside"
      const activeDrawer = app.classList.contains('workspace-mode')
        ? $('.workspace-sidebar')
        : $('.sidebar');
      if (activeDrawer && !activeDrawer.contains(e.target) && !toggleBtn.contains(e.target)) {
        app.classList.remove('sidebar-open');
        syncToggleLabels();
      }
    });
    window.addEventListener('resize', syncToggleLabels);
  }

  // In-sidebar collapse button (expanded desktop state)
  if (collapseBtn) {
    collapseBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      toggleCollapse();
    });
  }

  // Sidebar logo — when collapsed (desktop), click expands; otherwise
  // navigates home. Takes precedence over the generic [data-view-link].
  if (sidebarLogo) {
    sidebarLogo.addEventListener('click', (e) => {
      if (app?.classList.contains('sidebar-collapsed') && !isMobile()) {
        e.preventDefault();
        e.stopPropagation();
        toggleCollapse();
      } else {
        switchView('home');
      }
    });
  }

  // Chat history drawer toggle (inside AI assistant chat view)
  const chatHistoryToggle = $('#chat-history-toggle-btn');
  if (chatHistoryToggle) {
    chatHistoryToggle.addEventListener('click', (e) => {
      e.stopPropagation();
      $('.chat-layout')?.classList.toggle('chat-history-open');
    });
    // Close on backdrop click or when picking a history item
    document.addEventListener('click', (e) => {
      const layout = $('.chat-layout');
      if (!layout?.classList.contains('chat-history-open')) return;
      const sidebar = $('.chat-sidebar');
      const clickedInside = sidebar?.contains(e.target);
      const clickedToggle = chatHistoryToggle.contains(e.target);
      const clickedItem = e.target.closest('.chat-history-item');
      if ((!clickedInside && !clickedToggle) || clickedItem) {
        layout.classList.remove('chat-history-open');
      }
    });
  }
}

// ---------- Form application modal (with approval workflow) ----------
const FORM_WORKFLOW = [
  {
    title: '提交申請', status: 'completed',
    statusText: '已完成', meta: '剛剛',
    person: { name: 'Tammy Chen', role: '產品經理 · PD-Dev', initial: 'T' }
  },
  {
    title: '直屬主管審核', status: 'current',
    statusText: '進行中 · 預計 1 工作天內處理', meta: '',
    person: { name: 'Steve Lin', role: 'Engineering Director', initial: 'S' }
  },
  {
    title: 'HR 複核', status: 'pending',
    statusText: '待處理', meta: '',
    person: { name: '陳育恩', role: 'HR Manager', initial: '陳' }
  },
  {
    title: '總經理核准', status: 'pending',
    statusText: '待處理', meta: '',
    person: { name: 'Sega Cheng', role: 'CEO', initial: 'S' }
  },
];

// Render the form-modal's right-side workflow from the builder's _fbStages
// (used in preview mode so the panel matches the designed flow exactly).
const _FB_STAGE_MOCK_PERSON = {
  manager:    '林小琪',
  'dept-head':'張琪',
  hr:         '張文君',
  finance:    '吳勝利',
  ceo:        '王偉',
  applicant:  '陳怡如',
};
// 申請者等級 +N 主管 → mock person (prototype lookup)
const _FB_RELATIVE_MOCK_BY_LEVEL = { 1: '林小琪', 2: '張琪', 3: '王偉' };

function renderWorkflowFromStages(stages) {
  const root = $('#workflow-steps');
  if (!root) return;
  // Hide condition branches in the preview — they're flow-control nodes,
  // not actual review steps an applicant cares about.
  const visible = (stages || []).filter(s => s.type !== 'condition');
  if (visible.length === 0) {
    root.innerHTML = '<div class="p-small muted">尚未設定審核流程</div>';
    return;
  }
  const mockPerson = (s) => {
    if (s.role === 'custom' && s.personId) {
      const p = (typeof ORG_DATA !== 'undefined' ? ORG_DATA : []).find(r => r.id === s.personId);
      return p ? p.nameZh : '—';
    }
    if (s.role === 'relative') {
      const n = Number(s.relativeLevel) || 1;
      return _FB_RELATIVE_MOCK_BY_LEVEL[n] || _FB_RELATIVE_MOCK_BY_LEVEL[3];
    }
    return _FB_STAGE_MOCK_PERSON[s.role] || '—';
  };

  const steps = visible.map((s, i) => {
    let title;
    let person = mockPerson(s);
    if (s.type === 'approver') {
      title = `${_fbStageDisplay(s)} 審核`;
    } else if (s.type === 'notify') {
      title = `知會 ${_fbStageDisplay(s)}`;
    } else if (s.type === 'parallel') {
      title = `${_fbStageDisplay({ role: s.role })} 會簽`;
    } else {
      title = '步驟';
    }
    return `
      <div class="workflow-step pending">
        <div class="workflow-step-dot">${i + 1}</div>
        <div class="workflow-step-content">
          <div class="workflow-step-title">${title}</div>
          <div class="workflow-step-person">${person}</div>
        </div>
      </div>
    `;
  });

  // Trailing 完成 / 流程結束 node — mirrors the builder's right-side preview
  steps.push(`
    <div class="workflow-step pending workflow-step-end">
      <div class="workflow-step-dot"><i data-lucide="check" class="icon"></i></div>
      <div class="workflow-step-content">
        <div class="workflow-step-title">完成</div>
        <div class="workflow-step-person">流程結束</div>
      </div>
    </div>
  `);
  root.innerHTML = steps.join('');
  iconsRefresh();
}

function renderWorkflowSteps(finalStatus = null) {
  const root = $('#workflow-steps');
  if (!root) return;
  root.innerHTML = FORM_WORKFLOW.map((step, i) => {
    const isLast = i === FORM_WORKFLOW.length - 1;
    let status = step.status;
    let dotInner;
    if (finalStatus) {
      if (isLast && finalStatus !== 'approved') {
        status = finalStatus; // 'returned' or 'rejected'
        dotInner = finalStatus === 'rejected'
          ? `<i data-lucide="x" class="icon"></i>`
          : `<i data-lucide="corner-up-left" class="icon"></i>`;
      } else {
        status = 'completed';
        dotInner = `<i data-lucide="check" class="icon"></i>`;
      }
    } else {
      dotInner = status === 'completed'
        ? `<i data-lucide="check" class="icon"></i>`
        : status === 'current'
          ? `<i data-lucide="loader" class="icon"></i>`
          : `${i + 1}`;
    }
    return `
      <div class="workflow-step ${status}">
        <div class="workflow-step-dot">${dotInner}</div>
        <div class="workflow-step-content">
          <div class="workflow-step-title">${step.title}</div>
          <div class="workflow-step-person">${step.person.name}</div>
        </div>
      </div>
    `;
  }).join('');
}

// Defaults for the create-mode form (Tammy's leave application sample)
const FORM_DEFAULTS = {
  'applicant-name': 'Tammy Chen',
  'applicant-id':   'IKL020',
  'applicant-dept': '產品開發部 PD-Dev',
  'applicant-role': 'Product Manager',
  'start-date':     '2026/04/20',
  'end-date':       '2026/04/20',
  'period':         '全天',
  'reason':         '家人從國外回來探親，希望能陪伴一天。',
  'delegate':       'Ella Wang（已同意代理）',
};

function setFormValue(field, value) {
  const el = document.querySelector(`#form-modal-form [data-field="${field}"]`);
  if (el) el.value = value;
}

function renderReviewLog(data) {
  const root = $('#review-log');
  if (!root) return;
  const log = data?.reviewLog || [];
  if (log.length === 0) {
    root.innerHTML = `<div class="review-log-empty">尚未有他人審核紀錄，你是第一位審核者</div>`;
    return;
  }
  const labels = { approve: '核准', return: '退回補件', reject: '不通過' };
  root.innerHTML = log.map(entry => {
    const initial = (entry.name || '').slice(0, 1);
    const cls = entry.type === 'approve' ? 'approve' : entry.type === 'return' ? 'return' : 'reject';
    const actionLabel = labels[entry.type] || entry.type;
    const commentText = entry.comment && entry.comment.trim() ? entry.comment : '無意見';
    const commentMuted = (!entry.comment || !entry.comment.trim()) ? ' review-log-comment-empty' : '';
    return `
      <div class="review-log-item">
        <div class="review-log-avatar">${initial}</div>
        <div class="review-log-body">
          <div class="review-log-head">
            <span class="review-log-name">${entry.name}</span>
            <span class="review-log-role">· ${entry.role}</span>
            <span class="review-log-action ${cls}">${actionLabel}</span>
          </div>
          <div class="review-log-time">${fmtDate(entry.time)}</div>
          <div class="review-log-comment${commentMuted}">${commentText}</div>
        </div>
      </div>
    `;
  }).join('');
}

function openFormModal(title, mode = 'create', data = null) {
  const modal = $('#form-modal');
  if (!modal) return;
  if (title) $('#form-modal-title').textContent = title;
  const emojiEl = $('#form-modal-emoji');
  if (emojiEl) emojiEl.textContent = NOTIF_EMOJI_BY_TITLE[title] || _fbBasic?.icon || '📝';

  // Clear any persistent review-result overlay from a previous session so the
  // form body shows when this modal opens the next pending item
  const prevOverlay = document.getElementById('form-modal-success');
  if (prevOverlay) {
    prevOverlay.hidden = true;
    prevOverlay.classList.remove('form-modal-success-final');
    prevOverlay.querySelector('.form-modal-success-actions')?.remove();
    prevOverlay.querySelector('.form-modal-success-close')?.remove();
  }

  // On mobile, nest the workflow card inside .form-modal-form so it sits
  // visually inside the form body (above 申請人資訊). On desktop, restore it
  // as a sibling so the 2-column grid layout works.
  const workflowEl = modal.querySelector('.form-modal-workflow');
  const formEl = modal.querySelector('.form-modal-form');
  const bodyEl = modal.querySelector('.form-modal-body');
  if (workflowEl && formEl && bodyEl) {
    const isMobile = window.matchMedia('(max-width: 720px)').matches;
    if (isMobile && workflowEl.parentElement !== formEl) {
      formEl.insertBefore(workflowEl, formEl.firstChild);
    } else if (!isMobile && workflowEl.parentElement !== bodyEl) {
      bodyEl.appendChild(workflowEl);
    }
  }

  // Toggle classes on the inner .form-modal dialog (not the overlay)
  const dialog = modal.querySelector('.form-modal');
  const isReviewLike = mode === 'review' || mode === 'reviewed';
  const isPreview = mode === 'preview';
  // Reapply applies only to 已退回 / 已取消 forms (the user can resubmit them)
  const needsReapply = mode === 'reviewed' && data &&
    (data.status === 'destructive' || data.status === 'secondary');

  // Forms in DYNAMIC_FORMS use the builder-designed schema in every mode.
  // Auto-seed if not yet populated, or re-seed when switching between forms.
  if (DYNAMIC_FORMS.has(title) && (_fbFields.length === 0 || _fbBasic.name !== title)) {
    const meta = (title === '加班核准申請單' || title === '加班申請單')
      ? { category: '人事考勤類', icon: '⏰', desc: '*平日加班請勿超過4小時' }
      : { category: '人事考勤類', icon: '🗓️', desc: '' };
    _fbBasic = { name: title, ...meta };
    _fbSeedFormByName(title);
  }
  const useDynamic = DYNAMIC_FORMS.has(title) && _fbFields.length > 0;

  const isParallelSign = isReviewLike && data?.signType === 'parallel';
  if (dialog) {
    dialog.classList.toggle('form-modal-review', isReviewLike);
    dialog.classList.toggle('form-modal-reviewed', mode === 'reviewed');
    dialog.classList.toggle('form-modal-reviewed-reapply', needsReapply);
    // Only saved drafts show the delete button; brand-new forms don't.
    dialog.classList.toggle('form-modal-draft', mode === 'draft');
    dialog.classList.toggle('form-modal-preview', isPreview);
    dialog.classList.toggle('form-modal-dynamic', useDynamic);
    // View mode (opened from the 已申請 list) — destructive cancel + duplicate
    dialog.classList.toggle('form-modal-view', mode === 'view');
    // 待會簽 — show parallel-sign footer (退回 / 會簽) instead of the 3-btn review
    dialog.classList.toggle('form-modal-parallel-sign', isParallelSign);
    // Pre-submit preview is a transient state inside a single create session
    // — always clear it on (re)open so re-opening a form in any mode doesn't
    // surface the 返回修改 / 確定送出 footer left over from a previous run.
    dialog.classList.remove('form-modal-confirm-submit');
  }

  // Render _fbFields into the dynamic host whenever the form has a dynamic
  // schema. Otherwise clear the host so the static sections show through.
  const dynHost = modal.querySelector('[data-form-leave-dynamic]');
  if (dynHost) {
    if (useDynamic) {
      const topLevel = _fbFields.filter(f => !f.parentRow);
      dynHost.innerHTML = topLevel.map(f => _fbBuildLivePreviewField(f)).join('');
    } else {
      dynHost.innerHTML = '';
    }
  }

  const subtitleEl = $('#form-modal-subtitle');
  // Show 編號 / 建立時間 row in both review and preview modes
  if (subtitleEl) subtitleEl.hidden = !(isReviewLike || isPreview);
  if (isPreview) {
    const noEl = $('#form-modal-no'); if (noEl) noEl.textContent = 'NX-DRAFT';
    const subTimeEl = $('#form-modal-sub-time');
    if (subTimeEl) subTimeEl.textContent = '建立時間：—';
  }
  let workflowFinalStatus = null;

  if (isReviewLike && data) {
    // Parse applicant info from data.who (e.g. "王小明（Engineering）")
    const m = (data.who || '').match(/^(.+?)[（(](.+?)[）)]$/);
    const name = m?.[1] || data.who || '';
    const dept = m?.[2] || '';
    setFormValue('applicant-name', name);
    setFormValue('applicant-dept', dept);
    setFormValue('applicant-id', 'IKL020');  // placeholder
    setFormValue('applicant-role', '—');
    if (data.desc) setFormValue('reason', data.desc);
    // Lock all editable inputs
    modal.querySelectorAll('[data-editable]').forEach(el => el.setAttribute('readonly', ''));
    $('#form-modal-no').textContent = data.formNo || 'NX-2026-0421';
    const subTimeEl = $('#form-modal-sub-time');
    if (subTimeEl) subTimeEl.textContent = `建立時間：${fmtDateLong(data.time) || '—'}`;
    // Reset reviewer comment for each open
    const commentEl = $('#form-review-comment');
    if (commentEl) commentEl.value = '';
    // Render prior review log
    renderReviewLog(data);
    // Map REVIEWED status → workflow final status
    if (mode === 'reviewed') {
      workflowFinalStatus =
        data.status === 'destructive' ? 'rejected' :
        data.status === 'secondary'   ? 'returned' :
                                        'approved';
    }
  } else {
    // Create mode — restore defaults and editability (no form-no / 建立時間 yet)
    Object.entries(FORM_DEFAULTS).forEach(([k, v]) => setFormValue(k, v));
    modal.querySelectorAll('[data-editable]').forEach(el => el.removeAttribute('readonly'));
  }

  if (useDynamic && _fbStages.length) {
    renderWorkflowFromStages(_fbStages);
  } else {
    renderWorkflowSteps(workflowFinalStatus);
  }
  const summaryEl = $('#workflow-summary');
  if (summaryEl) {
    if (mode === 'reviewed') {
      summaryEl.textContent =
        workflowFinalStatus === 'rejected' ? '已不通過' :
        workflowFinalStatus === 'returned' ? '已退回' :
                                             '已核准';
    } else if (useDynamic && _fbStages.length) {
      const visibleCount = _fbStages.filter(s => s.type !== 'condition').length;
      summaryEl.textContent = `共 ${visibleCount} 關`;
    } else {
      summaryEl.textContent = '共 4 關';
    }
  }
  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  iconsRefresh();
}

function closeFormModal() {
  const modal = $('#form-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
  // Clean up any review session state so future opens start fresh
  reviewSession = null;
  // The pre-submit preview is a transient sub-state — never carry it across
  // an explicit close, otherwise the next open could surface the wrong footer
  modal.querySelector('.form-modal')?.classList.remove('form-modal-confirm-submit');
  const overlay = document.getElementById('form-modal-success');
  if (overlay) {
    overlay.hidden = true;
    overlay.classList.remove('form-modal-success-final');
    overlay.querySelector('.form-modal-success-close')?.remove();
  }
}

// On mobile, the form-modal header gets crowded:
//   * 建立時間 stays under the title (inside the subtitle row)
//   * 編號 + 列印/下載 按鈕 move above the approval workflow
// On desktop the 編號 span + actions return to their original locations.
function applyFormModalMobileLayout() {
  const modal = $('#form-modal');
  if (!modal) return;
  const noLine = $('#form-modal-no-line');
  const printBtn = $('#form-modal-print');
  const downloadBtn = $('#form-modal-download');
  const closeBtn = $('#form-modal-close');
  const subtitleMeta = modal.querySelector('.form-modal-subtitle-meta');
  const headActions = modal.querySelector('.form-modal-head-actions');
  const workflow = modal.querySelector('.form-modal-workflow');
  if (!noLine || !printBtn || !downloadBtn || !workflow || !subtitleMeta || !headActions) return;

  const isMobile = window.matchMedia('(max-width: 720px)').matches;
  let mobileMeta = workflow.querySelector('.form-modal-workflow-mobile-meta');

  if (isMobile) {
    if (!mobileMeta) {
      mobileMeta = document.createElement('div');
      mobileMeta.className = 'form-modal-workflow-mobile-meta';
      workflow.prepend(mobileMeta);
    }
    if (noLine.parentElement !== mobileMeta) mobileMeta.appendChild(noLine);
    let mobileActions = mobileMeta.querySelector('.form-modal-workflow-mobile-actions');
    if (!mobileActions) {
      mobileActions = document.createElement('div');
      mobileActions.className = 'form-modal-workflow-mobile-actions';
      mobileMeta.appendChild(mobileActions);
    }
    if (printBtn.parentElement !== mobileActions) mobileActions.appendChild(printBtn);
    if (downloadBtn.parentElement !== mobileActions) mobileActions.appendChild(downloadBtn);
  } else {
    // Desktop — put 編號 back as the first child of subtitle-meta, restore
    // the print/download buttons in the header before the close button.
    if (noLine.parentElement !== subtitleMeta) subtitleMeta.insertBefore(noLine, subtitleMeta.firstChild);
    if (closeBtn) {
      if (printBtn.parentElement !== headActions) headActions.insertBefore(printBtn, closeBtn);
      if (downloadBtn.parentElement !== headActions) headActions.insertBefore(downloadBtn, closeBtn);
    }
    if (mobileMeta && !mobileMeta.querySelector('#form-modal-no-line')
      && !mobileMeta.querySelector('#form-modal-print')
      && !mobileMeta.querySelector('#form-modal-download')) {
      mobileMeta.remove();
    }
  }
}

function initFormModal() {
  const modal = $('#form-modal');
  if (!modal) return;

  applyFormModalMobileLayout();
  window.addEventListener('resize', applyFormModalMobileLayout);

  $('#form-modal-close')?.addEventListener('click', closeFormModal);
  $('#form-modal-cancel')?.addEventListener('click', closeFormModal);
  $('#form-modal-cancel-review')?.addEventListener('click', closeFormModal);
  $('#form-modal-print')?.addEventListener('click', () => window.print());
  $('#form-modal-download')?.addEventListener('click', () => {
    // Placeholder — hook up to real download endpoint when API is available.
  });
  $('#form-modal-delete-btn')?.addEventListener('click', () => {
    const title = $('#form-modal-title')?.textContent || '此草稿';
    confirmDialog({
      title: '確定要刪除此草稿？',
      desc: `「${title}」將被移除，此動作無法復原。`,
      confirmText: '刪除',
      onConfirm: closeFormModal,
    });
  });
  modal.addEventListener('click', (e) => { if (e.target === modal) closeFormModal(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && modal.classList.contains('open')) closeFormModal();
  });

  // Selection chips (for 假別) — toggle behaviour
  modal.addEventListener('click', (e) => {
    const chip = e.target.closest('.selection-chip');
    if (!chip) return;
    const group = chip.parentElement;
    group.querySelectorAll('.selection-chip').forEach(c => c.classList.remove('selected'));
    chip.classList.add('selected');
    const input = chip.querySelector('input');
    if (input) input.checked = true;
  });

  // Reapply (for returned / withdrawn / rejected forms)
  $('#form-modal-reapply-btn')?.addEventListener('click', () => {
    const title = $('#form-modal-title')?.textContent || '申請單';
    closeFormModal();
    openFormModal(title, 'create');
  });

  // View mode — 取消申請: confirm before pulling the request back
  $('#form-modal-cancel-app-btn')?.addEventListener('click', () => {
    const title = $('#form-modal-title')?.textContent || '此申請';
    confirmDialog({
      title: '確定要取消申請？',
      desc: `「${title}」將被撤回,此動作無法復原。`,
      confirmText: '取消申請',
      onConfirm: () => {
        closeFormModal();
        showToast({ title: '已取消申請', desc: `${title} 已取消`, variant: 'info' });
      },
    });
  });

  // View mode — 複製表單: reopen this form as a brand-new draft
  $('#form-modal-duplicate-btn')?.addEventListener('click', () => {
    const title = $('#form-modal-title')?.textContent || '申請單';
    closeFormModal();
    openFormModal(title, 'create');
    showToast({ title: '已複製表單', desc: `已用「${title}」內容開啟新申請`, variant: 'info' });
  });

  // Submit (新申請單送出) — show a confirm preview first so the user can
  // sanity-check what's about to be sent. Real submission happens only after
  // they click 確定送出 in the preview footer.
  $('#form-modal-submit-btn')?.addEventListener('click', () => {
    const dialog = modal.querySelector('.form-modal');
    if (!dialog) return;
    // Piggyback on form-modal-review for the horizontal preview styling;
    // form-modal-confirm-submit hides the reviewer-only section and swaps
    // the footer.
    dialog.classList.add('form-modal-review', 'form-modal-confirm-submit');
    modal.querySelector('.modal-body')?.scrollTo?.({ top: 0, behavior: 'smooth' });
  });

  // Preview → 返回修改: exit confirm mode, restore the editable form
  $('#form-modal-confirm-back-btn')?.addEventListener('click', () => {
    const dialog = modal.querySelector('.form-modal');
    dialog?.classList.remove('form-modal-review', 'form-modal-confirm-submit');
  });

  // Preview → 確定送出: actual submission flow (success overlay → toast)
  $('#form-modal-confirm-submit-btn')?.addEventListener('click', async () => {
    const title = $('#form-modal-title')?.textContent || '申請單';
    // Pick up the first approver from the workflow steps so the toast can be
    // specific. Falls back to "主管" if none found.
    const firstApprover = $('#workflow-steps .workflow-step .workflow-step-name')?.textContent
      || $('#workflow-steps .workflow-step-title')?.textContent
      || '主管';
    await playFormModalSuccess({ title: '已送出', desc: `${title} 已成功送出` });
    closeFormModal();
    showToast({
      title: '已送出',
      desc: `已通知 ${firstApprover}`,
      variant: 'success',
    });
  });

  // Save draft — show a quick toast without the in-modal overlay
  $('#form-modal-save-draft-btn')?.addEventListener('click', () => {
    closeFormModal();
    showToast({ title: '已存為草稿', desc: '可在「草稿區」繼續編輯', variant: 'info' });
  });

  // Review actions — show a completion screen inside the modal. User chooses
  // whether to end the review session or continue to the next pending item.
  modal.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-review-action]');
    if (!btn) return;
    const action = btn.getAttribute('data-review-action');
    // In 待會簽 mode (parallel-sign) the same actions narrate differently:
    // approve → 已會簽, return → 已退回. Outside that mode the standard
    // approve / reject / return labels apply.
    const isParallelMode = modal.querySelector('.form-modal.form-modal-parallel-sign');
    let actionLabel;
    if (isParallelMode) {
      actionLabel = action === 'approve' ? '已會簽' : '已退回';
    } else {
      actionLabel = action === 'approve'
        ? '已核准' : action === 'reject'
        ? '已標記不通過' : '已退回補件';
    }

    // Mark the current item as reviewed by bumping the session index; the
    // "next" decision is the user's via the continuation prompt.
    if (reviewSession) reviewSession.index += 1;
    const next = reviewSession?.queue[reviewSession.index];
    showReviewResult({ actionLabel, next });
  });
}

// ---------- Review session (auto-advance through pending queue) ----------
let reviewSession = null;  // { queue: [pending items], index: number }

function startReviewSession(startItem) {
  // Build queue from PENDING (still-pending items). Keep the clicked item
  // at the front so the modal opens on it, then proceed in list order.
  const queue = [...PENDING];
  const startIdx = queue.findIndex(n => n === startItem);
  reviewSession = { queue, index: startIdx >= 0 ? startIdx : 0 };
}

/**
 * Show a persistent review-result screen inside the form modal with user-
 * chosen next step. `next` is the next pending item (or undefined when the
 * queue is cleared). Never auto-advances — user always picks.
 */
function showReviewResult({ actionLabel, next }) {
  const original = document.getElementById('form-modal-success');
  if (!original) return;

  // Clone to restart the keyframe animations on a fresh element
  const overlay = original.cloneNode(true);
  original.parentNode.replaceChild(overlay, original);

  const titleEl = overlay.querySelector('#form-modal-success-title');
  const descEl = overlay.querySelector('#form-modal-success-desc');
  if (next) {
    if (titleEl) titleEl.textContent = '已審核';
    if (descEl) descEl.textContent = `${actionLabel} · 還有 ${reviewSession.queue.length - reviewSession.index} 件待處理`;
  } else {
    if (titleEl) titleEl.textContent = '你已審核完畢';
    if (descEl) descEl.innerHTML = '所有待處理項目都處理完了 🎉';
  }

  overlay.hidden = false;
  overlay.classList.add('form-modal-success-final');

  // Build action button row
  const actions = el('div', 'form-modal-success-actions');
  if (next) {
    const endBtn = el('button', 'btn btn-outline form-modal-success-btn', '結束審核');
    const nextBtn = el('button', 'btn btn-primary form-modal-success-btn', '繼續下一筆');
    endBtn.addEventListener('click', closeFormModal);
    nextBtn.addEventListener('click', () => {
      openFormModal(next.title, 'review', next);
    });
    actions.appendChild(endBtn);
    actions.appendChild(nextBtn);
  } else {
    const closeBtn = el('button', 'btn btn-primary form-modal-success-btn', '關閉');
    closeBtn.addEventListener('click', closeFormModal);
    actions.appendChild(closeBtn);
    reviewSession = null;
  }
  overlay.appendChild(actions);
  iconsRefresh();
}

// ---------- All-notifications modal ----------
let currentNotifFilter = 'all';

function updateNotifFilterCounts() {
  const counts = { all: SYSTEM_NOTIFS.length, success: 0, info: 0, warning: 0 };
  SYSTEM_NOTIFS.forEach(n => { if (counts[n.status] != null) counts[n.status]++; });
  Object.entries(counts).forEach(([key, val]) => {
    const el = document.querySelector(`#notif-filter-tabs [data-count="${key}"]`);
    if (el) el.textContent = val;
  });
}

function renderAllNotifsModal(filter = currentNotifFilter) {
  currentNotifFilter = filter;
  const body = $('#all-notifs-body');
  const count = $('#all-notifs-count');
  if (!body) return;

  const items = filter === 'all'
    ? SYSTEM_NOTIFS
    : SYSTEM_NOTIFS.filter(n => n.status === filter);

  body.innerHTML = '';
  if (items.length === 0) {
    body.innerHTML = `
      <div class="empty-state">
        <i data-lucide="bell-off" class="icon"></i>
        <div class="h4">此分類目前沒有通知</div>
      </div>
    `;
  } else {
    items.forEach(n => body.appendChild(buildNotifItem(n, 'system')));
  }
  if (count) count.textContent = `共 ${items.length} 筆通知`;
  updateNotifFilterCounts();
}

function initAllNotifsModal() {
  const modal = $('#all-notifs-modal');
  const openBtn = $('#view-all-notifs-btn');
  const closeBtn = $('#all-notifs-close');
  const notifDD = $('#notif-dropdown');
  if (!modal || !openBtn) return;

  const open = () => {
    renderAllNotifsModal();
    modal.classList.add('open');
    modal.setAttribute('aria-hidden', 'false');
    notifDD?.classList.remove('open');
    iconsRefresh();
  };
  const close = () => {
    modal.classList.remove('open');
    modal.setAttribute('aria-hidden', 'true');
  };

  openBtn.addEventListener('click', (e) => { e.stopPropagation(); open(); });
  closeBtn?.addEventListener('click', close);
  // Click on overlay (outside .modal) closes
  modal.addEventListener('click', (e) => { if (e.target === modal) close(); });
  // ESC closes
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && modal.classList.contains('open')) close();
  });

  // Filter tabs
  $$('#notif-filter-tabs .notif-filter-tab').forEach(tab => {
    tab.addEventListener('click', () => {
      $$('#notif-filter-tabs .notif-filter-tab').forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
      renderAllNotifsModal(tab.getAttribute('data-filter'));
      iconsRefresh();
    });
  });
}

// ---------- Dropdowns (user menu + notification bell) ----------
function initDropdown() {
  const userDD = $('#user-dropdown');
  const avatar = $('#user-avatar');
  avatar.addEventListener('click', (e) => {
    e.stopPropagation();
    toggleDropdownExclusive(userDD);
  });

  const notifDD = $('#notif-dropdown');
  const bell = $('#notif-bell-btn');
  bell.addEventListener('click', (e) => {
    e.stopPropagation();
    toggleDropdownExclusive(notifDD);
  });

  // Close both on outside click
  document.addEventListener('click', (e) => {
    if (!userDD.contains(e.target)) userDD.classList.remove('open');
    if (!notifDD.contains(e.target)) notifDD.classList.remove('open');
  });

  // Hide the red dot when there are no system notifications
  const dot = $('#notif-dot');
  if (dot && SYSTEM_NOTIFS.length === 0) dot.style.display = 'none';
}

// ==================== Shared fullscreen overlay helpers ====================
// Handles common a11y wiring for login / error / legal overlays:
//   - body scroll lock
//   - aria-hidden on #main-content so SR doesn't read the background
//   - focus restoration (previously-focused element regains focus on close)
//   - Esc close + Tab focus trap (global keydown on the topmost overlay)
// Each overlay registers how it should be closed via `_overlayCloseFn` so
// that Esc dispatches to the correct hide routine.

const _overlayPrevFocus = new WeakMap();
const _overlayCloseFn = new WeakMap();
const _overlayStack = [];

function _overlayFocusables(container) {
  const nodes = container.querySelectorAll(
    'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'
  );
  return Array.from(nodes).filter(el => {
    // Exclude elements inside [hidden] ancestors within the overlay
    let n = el;
    while (n && n !== container) {
      if (n.hidden) return false;
      n = n.parentElement;
    }
    // Must be rendered (offsetParent null = display:none or ancestor hidden)
    return el.offsetParent !== null || el === document.activeElement;
  });
}

function openOverlay(overlay, closeFn, { focusTarget } = {}) {
  if (!overlay) return;
  _overlayPrevFocus.set(overlay, document.activeElement);
  _overlayCloseFn.set(overlay, closeFn);
  overlay.hidden = false;
  document.body.classList.add('login-overlay-open');
  const main = document.getElementById('main-content');
  if (main) main.setAttribute('aria-hidden', 'true');
  _overlayStack.push(overlay);
  requestAnimationFrame(() => overlay.classList.add('visible'));
  setTimeout(() => {
    let el = typeof focusTarget === 'string' ? overlay.querySelector(focusTarget)
           : focusTarget instanceof Element ? focusTarget
           : null;
    if (!el) el = _overlayFocusables(overlay)[0];
    el?.focus();
  }, 80);
  if (window.lucide?.createIcons) lucide.createIcons();
}

function closeOverlay(overlay) {
  if (!overlay) return;
  overlay.classList.remove('visible');
  const idx = _overlayStack.indexOf(overlay);
  if (idx >= 0) _overlayStack.splice(idx, 1);
  if (!_overlayStack.length) {
    document.body.classList.remove('login-overlay-open');
    const main = document.getElementById('main-content');
    if (main) main.removeAttribute('aria-hidden');
  }
  setTimeout(() => {
    overlay.hidden = true;
    const prev = _overlayPrevFocus.get(overlay);
    _overlayPrevFocus.delete(overlay);
    _overlayCloseFn.delete(overlay);
    if (prev instanceof HTMLElement && document.contains(prev)) prev.focus?.();
  }, 200);
}

// Global keydown: Esc → close topmost, Tab → trap focus within topmost
function initOverlayKeyboard() {
  document.addEventListener('keydown', (e) => {
    const top = _overlayStack[_overlayStack.length - 1];
    if (!top) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      const close = _overlayCloseFn.get(top);
      if (typeof close === 'function') close();
      else closeOverlay(top);
      return;
    }
    if (e.key === 'Tab') {
      const focusables = _overlayFocusables(top);
      if (focusables.length === 0) { e.preventDefault(); return; }
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault(); last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault(); first.focus();
      }
    }
  });
}

// ---------- Logout + login/signup/forgot overlay ----------
function initAuthOverlay() {
  const overlay = $('#login-overlay');
  const logoutBtn = $('#logout-btn');
  if (!overlay || !logoutBtn) return;

  const AUTH_FOCUS_TARGET = {
    login: '#login-email',
    signup: '#signup-name',
    forgot: '#forgot-email'
  };
  const AUTH_TITLE_ID = {
    login: 'login-overlay-title-login',
    signup: 'login-overlay-title-signup',
    forgot: 'login-overlay-title-forgot'
  };

  function showAuthCard(mode) {
    overlay.querySelectorAll('[data-auth-card]').forEach(c => {
      c.hidden = c.dataset.authCard !== mode;
    });
    // Update aria-labelledby so screen readers read the active card's title
    if (AUTH_TITLE_ID[mode]) overlay.setAttribute('aria-labelledby', AUTH_TITLE_ID[mode]);
    if (mode === 'forgot') setForgotState('a');
    setTimeout(() => $(AUTH_FOCUS_TARGET[mode])?.focus(), 80);
    if (window.lucide?.createIcons) lucide.createIcons();
  }

  function setForgotState(state) {
    const card = overlay.querySelector('[data-auth-card="forgot"]');
    if (!card) return;
    card.querySelectorAll('[data-forgot-state]').forEach(s => {
      s.hidden = s.dataset.forgotState !== state;
    });
    if (state === 'b') {
      const email = $('#forgot-email')?.value?.trim() || 'you@ikala.ai';
      const target = card.querySelector('.login-forgot-email');
      if (target) target.textContent = email;
    }
    if (window.lucide?.createIcons) lucide.createIcons();
  }

  function showLogin(mode = 'login') {
    overlay.querySelectorAll('[data-auth-card]').forEach(c => {
      c.hidden = c.dataset.authCard !== mode;
    });
    if (AUTH_TITLE_ID[mode]) overlay.setAttribute('aria-labelledby', AUTH_TITLE_ID[mode]);
    if (mode === 'forgot') setForgotState('a');
    $('#user-dropdown')?.classList.remove('open');
    openOverlay(overlay, hideLogin, { focusTarget: AUTH_FOCUS_TARGET[mode] });
  }

  function hideLogin() { closeOverlay(overlay); }

  logoutBtn.addEventListener('click', (e) => {
    e.stopPropagation();
    showLogin('login');
  });

  // In-overlay switchers: login ↔ signup ↔ forgot
  overlay.addEventListener('click', (e) => {
    const switcher = e.target.closest('[data-auth-switch]');
    if (switcher) {
      e.preventDefault();
      showAuthCard(switcher.dataset.authSwitch);
    }
  });

  // Any SSO button → return to home view (simulated login)
  overlay.querySelectorAll('[data-sso]').forEach(btn => {
    btn.addEventListener('click', () => {
      hideLogin();
      switchView('home');
      if (typeof showToast === 'function') {
        showToast({ title: '已登入', variant: 'success' });
      }
    });
  });

  // Email/password login submit → home
  $('#login-submit-btn')?.addEventListener('click', (e) => {
    e.preventDefault();
    hideLogin();
    switchView('home');
  });

  // Signup submit → home (demo)
  $('#signup-submit-btn')?.addEventListener('click', (e) => {
    e.preventDefault();
    hideLogin();
    switchView('home');
    if (typeof showToast === 'function') {
      showToast({ title: '帳號建立成功', variant: 'success' });
    }
  });

  // Forgot form submit → state B (success)
  $('#forgot-form')?.addEventListener('submit', (e) => {
    e.preventDefault();
    setForgotState('b');
  });

  // "使用其他 email" → back to state A
  $('#forgot-retry-btn')?.addEventListener('click', () => {
    setForgotState('a');
    setTimeout(() => $('#forgot-email')?.focus(), 40);
  });

  // Signup password strength
  const pwInput = $('#signup-password');
  if (pwInput) pwInput.addEventListener('input', () => {
    const v = pwInput.value;
    let score = 0;
    if (v.length >= 8) score++;
    if (/[A-Z]/.test(v) && /[a-z]/.test(v)) score++;
    if (/\d/.test(v)) score++;
    if (/[^A-Za-z0-9]/.test(v)) score++;
    const bar = $('#signup-pw-strength');
    const hint = $('#signup-pw-hint');
    if (bar) bar.className = 'pw-strength s' + score;
    const labels = ['請輸入密碼', '密碼太弱', '密碼強度：中等', '密碼強度：良好', '密碼強度：很強'];
    if (hint) hint.textContent = labels[score] || '';
  });

  // Shared password visibility toggle (for any [data-pw-toggle] button)
  overlay.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-pw-toggle]');
    if (!btn) return;
    const input = btn.parentElement.querySelector('input');
    const icon = btn.querySelector('.icon');
    if (!input || !icon) return;
    const show = input.type === 'password';
    input.type = show ? 'text' : 'password';
    icon.setAttribute('data-lucide', show ? 'eye-off' : 'eye');
    btn.setAttribute('aria-label', show ? '隱藏密碼' : '顯示密碼');
    if (window.lucide?.createIcons) lucide.createIcons();
  });
}

// ---------- Error overlays (404 / 403) — engineer preview ----------
function initErrorOverlays() {
  const previews = document.querySelectorAll('[data-preview-error]');
  if (!previews.length) return;

  function showErrorOverlay(code) {
    const overlay = document.getElementById(`error-overlay-${code}`);
    if (!overlay) return;
    $('#user-dropdown')?.classList.remove('open');
    openOverlay(overlay, () => closeOverlay(overlay));
  }

  previews.forEach(item => {
    item.addEventListener('click', (e) => {
      e.stopPropagation();
      showErrorOverlay(item.dataset.previewError);
    });
  });

  document.querySelectorAll('.error-overlay').forEach(overlay => {
    overlay.addEventListener('click', (e) => {
      const closeBtn = e.target.closest('[data-error-close]');
      if (closeBtn) closeOverlay(overlay);
    });
  });
}

// ---------- Legal overlays (privacy policy / terms of service) ----------
function initLegalOverlays() {
  const overlays = document.querySelectorAll('.legal-overlay');
  if (!overlays.length) return;

  function show(which) {
    const overlay = document.getElementById(`legal-overlay-${which}`);
    if (!overlay) return;
    overlay.scrollTop = 0;
    $('#user-dropdown')?.classList.remove('open');
    openOverlay(overlay, () => closeOverlay(overlay), { focusTarget: '.legal-close' });
  }

  // Any [data-legal="privacy|terms"] link → open corresponding overlay
  document.addEventListener('click', (e) => {
    const link = e.target.closest('[data-legal]');
    if (!link) return;
    e.preventDefault();
    show(link.dataset.legal);
  });

  overlays.forEach(overlay => {
    overlay.addEventListener('click', (e) => {
      const closeBtn = e.target.closest('[data-legal-close]');
      if (closeBtn) closeOverlay(overlay);
    });
  });
}

// ---------- Toggle switches (preferences page) ----------
function initPrefToggles() {
  document.querySelectorAll('.toggle[role="switch"]').forEach(t => {
    const flip = () => {
      const next = t.getAttribute('aria-checked') === 'true' ? 'false' : 'true';
      t.setAttribute('aria-checked', next);
    };
    t.addEventListener('click', flip);
    t.addEventListener('keydown', e => {
      if (e.key === ' ' || e.key === 'Enter') { e.preventDefault(); flip(); }
    });
  });

  initThemeSwitcher();
  initSettingsModal();
  initWorkspaceNav();
}

// ---------- Workspace admin section navigation ----------
function initWorkspaceNav() {
  const view = $('#view-workspace');
  if (!view) return;
  const navItems = view.querySelectorAll('[data-workspace-section]');
  const sections = view.querySelectorAll('.workspace-section');
  navItems.forEach(item => {
    item.addEventListener('click', () => {
      const target = item.dataset.workspaceSection;
      navItems.forEach(i => i.classList.toggle('active', i === item));
      sections.forEach(s => { s.hidden = s.dataset.section !== target; });
      // Forms section: always re-enter at list view (not lingering on builder)
      if (target === 'forms') _showFormsView?.('list');
      view.querySelector('.workspace-main')?.scrollTo?.({ top: 0 });
      if (window.lucide?.createIcons) lucide.createIcons();
    });
  });

  initEmployeeManagement();
  initOverviewMonthNav();
  initOrganization();
  initAdminManagement();
  initFormBuilder();
}

// ---------- Form builder (3-step wizard) ----------
const FB_FIELD_TYPE_LABELS = {
  name:        '姓名',
  email:       'Email',
  text:        '單行文字',
  mask:        '格式化輸入',
  textarea:    '多行文字',
  address:     '地址',
  country:     '國家清單',
  number:      '數字',
  select:      '下拉選單',
  radio:       '單選',
  checkbox:    '勾選方塊',
  multilist:   '多選清單',
  url:         '網址',
  datetime:    '日期時間',
  image:       '圖片上傳',
  file:        '檔案上傳',
  html:        '自訂 HTML',
  phone:       '電話',
  divider:     '分隔線',
  'section-title': '分類標題',
  autofill:    '自動帶入',
  'employee-select': '員工下拉選單',
  'layout-1-1':   '2 欄佈局 1:1',
  'layout-2-1':   '2 欄佈局 2:1',
  'layout-1-2':   '2 欄佈局 1:2',
  'layout-1-1-1': '3 欄佈局 1:1:1',
};
// Type categories for property-panel logic
const FB_TYPES_WITH_PLACEHOLDER = new Set(['name', 'email', 'text', 'mask', 'textarea', 'number', 'address', 'url', 'phone']);
const FB_TYPES_WITH_DEFAULT = new Set(['name', 'email', 'text', 'mask', 'textarea', 'number', 'address', 'url', 'phone']);
const FB_TYPES_WITH_OPTIONS = new Set(['select', 'radio', 'checkbox', 'multilist', 'country']);

// Basic info collected via the modal before opening the builder
let _fbBasic = { name: '', category: '', icon: '📝', desc: '' };
let _fbFields = [];
let _fbStages = [];
let _fbFieldUid = 0;
let _fbSelectedFieldUid = null;

// Forms whose schema is driven by the builder (auto-seeded from a PDF spec
// when opened). When `title` matches and `_fbFields` is empty, openFormModal
// runs the matching seed function so the create/edit/preview view shows the
// same fields + workflow that the form designer renders.
const DYNAMIC_FORMS = new Set(['請假申請單', '加班核准申請單', '加班申請單']);

// Seed the builder with the HR-001 請假單 spec (from 請假單.pdf). Used when
// editing the 請假申請單 row in the forms list. Fields use the standard
// FB field types; layouts use layout-N-N rows with parentRow + slotIndex
// children. Stages reflect the PDF's 職級 3,4,5 approval flow.
function _fbSeedLeaveForm() {
  _fbFields = [];
  _fbFieldUid = 0;
  const newUid = () => ++_fbFieldUid;
  const push = (overrides) => {
    const u = newUid();
    _fbFields.push({
      uid: u, type: 'text', label: '', required: false,
      placeholder: '', defaultValue: '', options: '',
      ...overrides,
    });
    return u;
  };

  // ===== 申請人資料 =====
  push({ type: 'section-title', label: '申請人資料' });

  // 申請人 / 申請人員工編號
  const r1 = push({ type: 'layout-1-1' });
  push({ type: 'autofill', label: '申請人',         source: 'session.user.name',       placeholder: '登入者自動帶入', parentRow: r1, slotIndex: 0 });
  push({ type: 'autofill', label: '申請人員工編號', source: 'session.user.employeeId', placeholder: '依申請人自動帶入', parentRow: r1, slotIndex: 1 });

  // 部門 / 職稱
  const r2 = push({ type: 'layout-1-1' });
  push({ type: 'autofill', label: '部門', source: 'session.user.dept',  placeholder: '依申請人自動帶入', parentRow: r2, slotIndex: 0 });
  push({ type: 'autofill', label: '職稱', source: 'session.user.title', placeholder: '依申請人自動帶入', parentRow: r2, slotIndex: 1 });

  // 代理人 / 代理人員工編號
  const r3 = push({ type: 'layout-1-1' });
  push({ type: 'employee-select', label: '代理人', required: true, scope: 'all', parentRow: r3, slotIndex: 0 });
  push({ type: 'autofill', label: '代理人員工編號', source: 'proxy.employeeId', placeholder: '依代理人自動帶入', parentRow: r3, slotIndex: 1 });

  // ===== 申請內容 =====
  push({ type: 'section-title', label: '申請內容' });

  // 假勤名稱 (獨立一行)
  push({
    type: 'select', label: '假勤名稱', required: true,
    options: [
      '特休假', '事假', '全薪病假', '半薪病假', '家庭照顧假',
      '生理假', '婚假', '喪假', '八週產假', '陪產假',
      '產檢假', '公假', '補休假', '彈性休假', '外勤',
    ].join('\n'),
  });

  // 開始時間 / 結束時間
  const r4 = push({ type: 'layout-1-1' });
  push({ type: 'datetime', label: '開始時間', required: true, parentRow: r4, slotIndex: 0 });
  push({ type: 'datetime', label: '結束時間', required: true, parentRow: r4, slotIndex: 1 });

  // 已休 / 未休 / 請假時數
  const r5 = push({ type: 'layout-1-1-1' });
  push({ type: 'autofill', label: '已休時數', source: 'leave.hoursTaken',     placeholder: '0', parentRow: r5, slotIndex: 0 });
  push({ type: 'autofill', label: '未休時數', source: 'leave.hoursRemaining', placeholder: '0', parentRow: r5, slotIndex: 1 });
  push({ type: 'autofill', label: '請假時數', source: 'leave.hoursRequested', placeholder: '0', parentRow: r5, slotIndex: 2 });

  push({ type: 'textarea', label: '請假原因', required: true, placeholder: '請填寫請假事由' });
  push({ type: 'file',     label: '附加檔案', placeholder: '單檔上限 20MB' });

  // 簽核流程 — 職級 3,4,5 員工：HR 知會 → 部門主管簽核 → (請假時數 ≥ 40) 高階主管簽核
  const condUid = newUid();
  _fbStages = [
    { uid: newUid(), type: 'notify',    role: 'hr' },
    { uid: newUid(), type: 'approver',  role: 'dept-head' },
    { uid: condUid, type: 'condition', condField: 'hours', condOp: '>=', condValue: 40 },
    { uid: newUid(), type: 'approver',  role: 'relative',  relativeLevel: 2, parentCondition: condUid },
  ];
}

// Seed the builder with the 加班核准申請單 spec (from 加班核准申請單.pdf).
// 9 main fields + standard 申請人/部門 autofill block, plus an attachment row.
// Flow: HR 知會 → 部門主管簽核 → 申請者通知 (3 關)。
function _fbSeedOvertimeForm() {
  _fbFields = [];
  _fbFieldUid = 0;
  const newUid = () => ++_fbFieldUid;
  const push = (overrides) => {
    const u = newUid();
    _fbFields.push({
      uid: u, type: 'text', label: '', required: false,
      placeholder: '', defaultValue: '', options: '',
      ...overrides,
    });
    return u;
  };

  // ===== 申請人資料 =====
  push({ type: 'section-title', label: '申請人資料' });

  const r1 = push({ type: 'layout-1-1' });
  push({ type: 'autofill', label: '申請人',         source: 'session.user.name',       placeholder: '登入者自動帶入', parentRow: r1, slotIndex: 0 });
  push({ type: 'autofill', label: '申請人員工編號', source: 'session.user.employeeId', placeholder: '依申請人自動帶入', parentRow: r1, slotIndex: 1 });

  const r2 = push({ type: 'layout-1-1' });
  push({ type: 'autofill', label: '部門', source: 'session.user.dept',  placeholder: '依申請人自動帶入', parentRow: r2, slotIndex: 0 });
  push({ type: 'autofill', label: '職稱', source: 'session.user.title', placeholder: '依申請人自動帶入', parentRow: r2, slotIndex: 1 });

  // ===== 申請內容 =====
  push({ type: 'section-title', label: '申請內容' });

  // 加班類別 (獨立一行)
  push({
    type: 'select', label: '加班類別', required: true,
    options: ['平日加班', '假日加班', '國定假日加班'].join('\n'),
  });

  // 加班開始 / 加班結束 (一行兩欄)
  const r3 = push({ type: 'layout-1-1' });
  push({ type: 'datetime', label: '加班開始', required: true, parentRow: r3, slotIndex: 0 });
  push({ type: 'datetime', label: '加班結束', required: true, parentRow: r3, slotIndex: 1 });

  // 加班時數 / 加班折換方式 (一行兩欄)
  const r4 = push({ type: 'layout-1-1' });
  push({ type: 'autofill', label: '加班時數', required: true, source: 'overtime.hours', placeholder: '0', parentRow: r4, slotIndex: 0 });
  push({
    type: 'select', label: '加班折換方式', required: true,
    options: ['補休', '加班費'].join('\n'),
    parentRow: r4, slotIndex: 1,
  });

  // 加班專案 自己一行
  push({ type: 'text', label: '加班專案', required: true, placeholder: '例:NX-Q2 行銷活動' });

  // 加班原因
  push({ type: 'textarea', label: '加班原因', required: true, placeholder: '請說明加班事由 (平日加班請勿超過 4 小時)' });

  // 附加檔案
  push({ type: 'file', label: '附加檔案', placeholder: '單檔上限 20MB' });

  // 簽核流程 — PDF 三個職級共用:HR 知會 → 部門主管簽核 (流程結束時系統會自動通知申請者)
  _fbStages = [
    { uid: newUid(), type: 'notify',   role: 'hr' },
    { uid: newUid(), type: 'approver', role: 'relative', relativeLevel: 1 },
  ];
}

// Seed the builder with the 加班申請單 spec — a copy of 加班核准申請單 plus a
// 加班核准申請單編號 select that lets the user pick which previously-approved
// overtime form this claim relates to. The select sits on the same row as
// 加班類別 (first slot).
function _fbSeedOvertimeApplicationForm() {
  _fbFields = [];
  _fbFieldUid = 0;
  const newUid = () => ++_fbFieldUid;
  const push = (overrides) => {
    const u = newUid();
    _fbFields.push({
      uid: u, type: 'text', label: '', required: false,
      placeholder: '', defaultValue: '', options: '',
      ...overrides,
    });
    return u;
  };

  // ===== 申請人資料 =====
  push({ type: 'section-title', label: '申請人資料' });

  const r1 = push({ type: 'layout-1-1' });
  push({ type: 'autofill', label: '申請人',         source: 'session.user.name',       placeholder: '登入者自動帶入', parentRow: r1, slotIndex: 0 });
  push({ type: 'autofill', label: '申請人員工編號', source: 'session.user.employeeId', placeholder: '依申請人自動帶入', parentRow: r1, slotIndex: 1 });

  const r2 = push({ type: 'layout-1-1' });
  push({ type: 'autofill', label: '部門', source: 'session.user.dept',  placeholder: '依申請人自動帶入', parentRow: r2, slotIndex: 0 });
  push({ type: 'autofill', label: '職稱', source: 'session.user.title', placeholder: '依申請人自動帶入', parentRow: r2, slotIndex: 1 });

  // ===== 申請內容 =====
  push({ type: 'section-title', label: '申請內容' });

  // 加班核准申請單編號 + 加班類別 (一行兩欄)
  const r3 = push({ type: 'layout-1-1' });
  push({
    type: 'select', label: '加班核准申請單編號', required: true,
    options: ['OT-2026-0503', 'OT-2026-0502', 'OT-2026-0501', 'OT-2026-0420'].join('\n'),
    placeholder: '請選擇已核准的加班申請單',
    parentRow: r3, slotIndex: 0,
  });
  push({
    type: 'select', label: '加班類別', required: true,
    options: ['平日加班', '假日加班', '國定假日加班'].join('\n'),
    parentRow: r3, slotIndex: 1,
  });

  // 加班開始 / 加班結束 (一行兩欄)
  const r4 = push({ type: 'layout-1-1' });
  push({ type: 'datetime', label: '加班開始', required: true, parentRow: r4, slotIndex: 0 });
  push({ type: 'datetime', label: '加班結束', required: true, parentRow: r4, slotIndex: 1 });

  // 加班時數 / 加班折換方式 (一行兩欄)
  const r5 = push({ type: 'layout-1-1' });
  push({ type: 'autofill', label: '加班時數', required: true, source: 'overtime.hours', placeholder: '0', parentRow: r5, slotIndex: 0 });
  push({
    type: 'select', label: '加班折換方式', required: true,
    options: ['補休', '加班費'].join('\n'),
    parentRow: r5, slotIndex: 1,
  });

  // 加班專案 自己一行
  push({ type: 'text', label: '加班專案', required: true, placeholder: '例:NX-Q2 行銷活動' });

  // 加班原因
  push({ type: 'textarea', label: '加班原因', required: true, placeholder: '請說明加班事由 (平日加班請勿超過 4 小時)' });

  // 附加檔案
  push({ type: 'file', label: '附加檔案', placeholder: '單檔上限 20MB' });

  // 簽核流程 — 沿用加班核准單:HR 知會 → 部門主管簽核
  _fbStages = [
    { uid: newUid(), type: 'notify',   role: 'hr' },
    { uid: newUid(), type: 'approver', role: 'relative', relativeLevel: 1 },
  ];
}

// Dispatcher — pick the right seed for the form name
function _fbSeedFormByName(name) {
  if (name === '請假申請單') _fbSeedLeaveForm();
  else if (name === '加班核准申請單') _fbSeedOvertimeForm();
  else if (name === '加班申請單') _fbSeedOvertimeApplicationForm();
}

// Mock data the live-preview maps known autofill sources to, so the preview
// shows real-looking values for the logged-in user rather than blanks.
const _FB_LIVE_MOCK = {
  'session.user.name':        '陳怡如',
  'session.user.employeeId':  'IKL020',
  'session.user.dept':        '產品開發部',
  'session.user.title':       'Product Manager',
  'proxy.employeeId':         'IKL010',
  // 時數類預設留空,讓欄位顯示 placeholder「0」
  'leave.hoursTaken':         '',
  'leave.hoursRemaining':     '',
  'leave.hoursRequested':     '',
  'overtime.hours':           '',
};
const _FB_LIVE_DEFAULT_PROXY = '林小琪 (產品開發部)';

// Build a single-field HTML chunk used by the live-preview modal. Mirrors the
// canvas renderer but omits the designer chrome (drag handle, delete button,
// selection highlight) and renders real interactive form controls.
function _fbBuildLivePreviewField(f) {
  const label = _esc(f.label || '');
  const required = f.required ? ' <span class="required">*</span>' : '';
  const ph = _esc(f.placeholder || '');

  // Layout row → grid with nested children
  if (typeof f.type === 'string' && f.type.startsWith('layout-')) {
    const ratios = f.type.slice('layout-'.length).split('-').map(Number);
    const gridCols = ratios.map(r => `${r}fr`).join(' ');
    const children = _fbFields.filter(c => c.parentRow === f.uid);
    const slots = Array.from({ length: ratios.length }, (_, i) => {
      const child = children.find(c => c.slotIndex === i);
      return child ? _fbBuildLivePreviewField(child) : '<div></div>';
    }).join('');
    return `<div class="fb-live-row" style="display:grid;grid-template-columns:${gridCols};gap:var(--space-lg);align-items:start">${slots}</div>`;
  }

  if (f.type === 'section-title') {
    return `<div class="fb-preview-section-title">${label || '分類標題'}</div>`;
  }
  if (f.type === 'divider') {
    return `<div class="fb-preview-divider">${label || '段落分隔'}</div>`;
  }
  if (f.type === 'html') {
    return `<div class="form-field">${f.defaultValue || ''}</div>`;
  }

  // Autofill — show mock value when the source matches a known key.
  // Preview hides the zap icon (designer-only affordance).
  if (f.type === 'autofill') {
    const mockValue = _esc(_FB_LIVE_MOCK[f.source] ?? '');
    return `<div class="form-field">
      <label class="form-field-label">${label}${required}</label>
      <div class="input-wrap"><input class="input" readonly value="${mockValue}" placeholder="${ph || '系統自動帶入'}"></div>
    </div>`;
  }

  // Employee select — pre-select a sensible default (current user's supervisor)
  if (f.type === 'employee-select') {
    const members = (typeof ORG_DATA !== 'undefined' ? ORG_DATA : []).slice(0, 20);
    const opts = members.map(m => {
      const text = `${m.nameZh} (${m.dept || '—'})`;
      const selected = text === _FB_LIVE_DEFAULT_PROXY ? ' selected' : '';
      return `<option${selected}>${_esc(text)}</option>`;
    });
    opts.unshift('<option value="">— 未指派 —</option>');
    return `<div class="form-field">
      <label class="form-field-label">${label}${required}</label>
      <div class="input-wrap"><select class="input">${opts.join('')}</select></div>
    </div>`;
  }

  // Single-select dropdowns
  if (f.type === 'select' || f.type === 'country') {
    const options = (f.options || '').split('\n').filter(Boolean);
    const opts = ['<option value="">請選擇</option>']
      .concat(options.map(o => `<option>${_esc(o)}</option>`))
      .join('');
    return `<div class="form-field">
      <label class="form-field-label">${label}${required}</label>
      <div class="input-wrap"><select class="input">${opts}</select></div>
    </div>`;
  }

  // Radio group
  if (f.type === 'radio') {
    const options = (f.options || '').split('\n').filter(Boolean);
    const items = options.map((o, i) => `<label class="fb-live-radio"><input type="radio" name="r-${f.uid}" ${i === 0 ? 'checked' : ''}> <span>${_esc(o)}</span></label>`).join('');
    return `<div class="form-field">
      <label class="form-field-label">${label}${required}</label>
      <div class="fb-live-radio-group">${items}</div>
    </div>`;
  }

  // Checkbox group (inline)
  if (f.type === 'checkbox') {
    const options = (f.options || '').split('\n').filter(Boolean);
    const items = options.map(o => `<label class="fb-live-check"><input type="checkbox" class="checkbox"> <span>${_esc(o)}</span></label>`).join('');
    return `<div class="form-field">
      <label class="form-field-label">${label}${required}</label>
      <div class="fb-live-check-group">${items}</div>
    </div>`;
  }

  // Multi-select list (vertical)
  if (f.type === 'multilist') {
    const options = (f.options || '').split('\n').filter(Boolean);
    const items = options.map(o => `<label class="fb-live-check"><input type="checkbox" class="checkbox"> <span>${_esc(o)}</span></label>`).join('');
    return `<div class="form-field">
      <label class="form-field-label">${label}${required}</label>
      <div class="fb-live-multilist">${items}</div>
    </div>`;
  }

  // Textarea
  if (f.type === 'textarea') {
    return `<div class="form-field">
      <label class="form-field-label">${label}${required}</label>
      <textarea class="form-textarea" rows="3" placeholder="${ph}"></textarea>
    </div>`;
  }

  // Date/time
  if (f.type === 'datetime') {
    return `<div class="form-field">
      <label class="form-field-label">${label}${required}</label>
      <div class="input-wrap"><i data-lucide="calendar" class="icon"></i><input class="input" type="datetime-local"></div>
    </div>`;
  }

  // File / image
  if (f.type === 'file' || f.type === 'image') {
    return `<div class="form-field">
      <label class="form-field-label">${label}${required}</label>
      <label class="form-file-drop"><i data-lucide="upload-cloud" class="icon"></i><span>${ph || '點擊或拖曳檔案至此'}</span><input type="file" hidden></label>
    </div>`;
  }

  // Default: input type by mapping
  const inputType = ({ email: 'email', number: 'number', url: 'url', phone: 'tel' })[f.type] || 'text';
  return `<div class="form-field">
    <label class="form-field-label">${label}${required}</label>
    <div class="input-wrap"><input class="input" type="${inputType}" placeholder="${ph}"></div>
  </div>`;
}

function _fbOpenLivePreview() {
  const modal = document.getElementById('fb-preview-modal');
  const body = document.getElementById('fb-preview-modal-body');
  if (!modal || !body) return;
  document.getElementById('fb-preview-modal-emoji').textContent = _fbBasic.icon || '📝';
  document.getElementById('fb-preview-modal-title').textContent = _fbBasic.name || '表單預覽';
  const topLevel = _fbFields.filter(f => !f.parentRow);
  body.innerHTML = topLevel.length
    ? topLevel.map(f => _fbBuildLivePreviewField(f)).join('')
    : '<div class="p-small muted">尚未新增任何欄位</div>';
  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  if (window.lucide?.createIcons) lucide.createIcons();
}

function _fbCloseLivePreview() {
  const modal = document.getElementById('fb-preview-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
}

function _fbDefaultLabel(type, idx) {
  const map = {
    name: '姓名', email: 'Email', text: '單行文字',
    mask: '格式化輸入', textarea: '多行文字', address: '地址',
    country: '國家', number: '數字', select: '下拉選單',
    radio: '單選欄位', checkbox: '勾選欄位', multilist: '多選清單',
    url: '網址', datetime: '日期時間', image: '圖片上傳',
    file: '檔案上傳', html: '自訂 HTML', phone: '電話',
    divider: '段落標題',
    'section-title': '分類標題',
    autofill: '自動帶入欄位',
    'employee-select': '員工下拉',
  };
  return `${map[type] || '欄位'} ${idx}`;
}

function _fbAddField(type) {
  _fbFieldUid++;
  const idx = _fbFields.length + 1;
  const f = {
    uid: _fbFieldUid,
    type,
    label: _fbDefaultLabel(type, idx),
    required: false,
    placeholder: '',
    defaultValue: '',
    options: '',
  };
  // Sensible defaults for option-based fields
  if (['select', 'radio', 'checkbox', 'multilist'].includes(type)) {
    f.options = '選項 1\n選項 2\n選項 3';
  } else if (type === 'country') {
    f.options = '台灣\n日本\n韓國\n美國\n英國\n德國\n法國';
  } else if (type === 'autofill') {
    f.source = '';
    f.placeholder = '系統自動帶入';
  } else if (type === 'employee-select') {
    f.scope = 'all';
  }
  _fbFields.push(f);
  _fbSelectedFieldUid = f.uid;  // newly added field is auto-selected
  _fbRenderPreview();
  _fbRenderProperties();
}

function _fbAddLayoutRow(ratioStr) {
  // ratioStr like "1-1", "2-1", "1-2", "1-1-1"
  const ratios = ratioStr.split('-').map(Number);
  const n = ratios.length;
  _fbFieldUid++;
  _fbFields.push({
    uid: _fbFieldUid,
    type: `layout-${ratioStr}`,
    label: `${n} 欄佈局 ${ratios.join(':')}`,
    required: false,
    placeholder: '',
    defaultValue: '',
    options: '',
  });
  _fbSelectedFieldUid = _fbFieldUid;
  _fbRenderPreview();
  _fbRenderProperties();
}

function _esc(s) {
  return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function _fbRenderPreview() {
  const list = document.getElementById('fb-fields-list');
  if (!list) return;
  // Only top-level fields (those without parentRow) appear in the main list;
  // child fields render inside their parent column row's slots.
  const topLevel = _fbFields.filter(f => !f.parentRow);
  if (topLevel.length === 0) {
    list.innerHTML = '<div class="fb-fields-empty p-small muted">點擊左側欄位類型新增<br>新增後可點選下方預覽欄位編輯設定</div>';
    if (window.lucide?.createIcons) lucide.createIcons();
    return;
  }
  list.innerHTML = topLevel.map(f => _fbRenderPreviewField(f, false)).join('');
  if (window.lucide?.createIcons) lucide.createIcons();
}

function _fbRenderPreviewField(f, isChild = false) {
  const isSelected = f.uid === _fbSelectedFieldUid;
  let cls = `fb-preview-field${isSelected ? ' is-selected' : ''}${isChild ? ' is-child' : ''}`;
  const ph = _esc(f.placeholder || '');
  const dv = _esc(f.defaultValue || '');
  // Top-level fields are draggable + keyboard-reorderable (Alt+↑/↓).
  // Children inside slots are not reorderable (they're slot-positioned).
  const draggableAttr = isChild ? '' : ' draggable="true" tabindex="0"';
  const handleHtml = isChild ? '' :
    `<div class="fb-preview-handle" aria-label="拖曳調整順序"><i data-lucide="grip-vertical" class="icon"></i></div>`;
  const deleteBtn = `<button type="button" class="fb-preview-delete" data-fb-delete="${f.uid}" aria-label="刪除欄位"><i data-lucide="x" class="icon"></i></button>`;

  if (f.type === 'divider') {
    return `<div class="${cls}" data-fb-uid="${f.uid}"${draggableAttr}>
      ${handleHtml}
      <div class="fb-preview-content">
        <div class="fb-preview-divider">${_esc(f.label || '段落分隔')}</div>
      </div>
      ${deleteBtn}
    </div>`;
  }

  if (f.type === 'section-title') {
    return `<div class="${cls}" data-fb-uid="${f.uid}"${draggableAttr}>
      ${handleHtml}
      <div class="fb-preview-content">
        <div class="fb-preview-section-title">${_esc(f.label || '分類標題')}</div>
      </div>
      ${deleteBtn}
    </div>`;
  }

  if (f.type === 'employee-select') {
    const required = f.required ? ' <span class="required">*</span>' : '';
    const scopeLabel = f.scope === 'dept' ? '部門員工' : '全部員工';
    const members = (typeof ORG_DATA !== 'undefined' ? ORG_DATA : []).slice(0, 8);
    const opts = ['<option>— 未指派 —</option>']
      .concat(members.map(m => `<option>${_esc(m.nameZh)} (${_esc(m.dept || '—')})</option>`))
      .join('');
    return `<div class="${cls}" data-fb-uid="${f.uid}"${draggableAttr}>
      ${handleHtml}
      <div class="fb-preview-content">
        <label class="form-field-label">${_esc(f.label || '員工下拉')}${required}</label>
        <div class="input-wrap"><select class="input">${opts}</select></div>
        <div class="p-mini muted" style="margin-top:var(--space-2xs)">可選範圍:${scopeLabel}</div>
      </div>
      ${deleteBtn}
    </div>`;
  }

  if (f.type === 'autofill') {
    const required = f.required ? ' <span class="required">*</span>' : '';
    const src = _esc(f.source || '尚未設定 API 來源');
    return `<div class="${cls}" data-fb-uid="${f.uid}"${draggableAttr}>
      ${handleHtml}
      <div class="fb-preview-content">
        <label class="form-field-label">${_esc(f.label || '自動帶入欄位')}${required}</label>
        <div class="input-wrap fb-preview-autofill">
          <i data-lucide="zap" class="icon"></i>
          <input class="input" readonly value="" placeholder="${src}">
        </div>
      </div>
      ${deleteBtn}
    </div>`;
  }

  // Multi-column layout row — slots sized by configured ratio (e.g. 2:1)
  if (f.type.startsWith('layout-')) {
    const ratios = f.type.slice('layout-'.length).split('-').map(Number);
    const n = ratios.length;
    const gridCols = ratios.map(r => `${r}fr`).join(' ');
    const children = _fbFields.filter(c => c.parentRow === f.uid);
    const slotsHtml = Array.from({ length: n }, (_, i) => {
      const child = children.find(c => c.slotIndex === i);
      if (child) {
        return `<div class="fb-cols-slot is-filled" data-fb-cols-slot="${i}" data-fb-cols-uid="${f.uid}">${_fbRenderPreviewField(child, true)}</div>`;
      }
      return `<div class="fb-cols-slot" data-fb-cols-slot="${i}" data-fb-cols-uid="${f.uid}">拖入欄位</div>`;
    }).join('');
    return `
      <div class="${cls}" data-fb-uid="${f.uid}"${draggableAttr}>
        ${handleHtml}
        <div class="fb-preview-content">
          <div class="fb-cols-row" style="grid-template-columns:${gridCols}">${slotsHtml}</div>
        </div>
        ${deleteBtn}
      </div>
    `;
  }

  const required = f.required ? ' <span class="required">*</span>' : '';
  const labelHtml = `<label class="form-field-label">${_esc(f.label || '未命名欄位')}${required}</label>`;
  const opts = (f.options || '').split('\n').map(s => s.trim()).filter(Boolean);
  let inputHtml = '';
  switch (f.type) {
    case 'name':
      inputHtml = `<div class="input-wrap"><i data-lucide="user" class="icon"></i><input class="input" placeholder="${ph || '請輸入姓名'}" value="${dv}" disabled></div>`;
      break;
    case 'email':
      inputHtml = `<div class="input-wrap"><i data-lucide="mail" class="icon"></i><input class="input" type="email" placeholder="${ph || 'name@example.com'}" value="${dv}" disabled></div>`;
      break;
    case 'text':
      inputHtml = `<div class="input-wrap"><input class="input" placeholder="${ph}" value="${dv}" disabled></div>`;
      break;
    case 'mask':
      inputHtml = `<div class="input-wrap"><i data-lucide="keyboard" class="icon"></i><input class="input" placeholder="${ph || 'A123-4567-8901'}" value="${dv}" disabled></div>`;
      break;
    case 'textarea':
      inputHtml = `<textarea class="form-textarea" rows="3" placeholder="${ph}" disabled>${dv}</textarea>`;
      break;
    case 'address':
      inputHtml = `<div class="input-wrap"><i data-lucide="map-pin" class="icon"></i><input class="input" placeholder="${ph || '縣市 / 鄉鎮市區 / 路段地址'}" value="${dv}" disabled></div>`;
      break;
    case 'country': {
      const list = opts.length ? opts : ['台灣', '日本', '韓國', '美國'];
      inputHtml = `<div class="input-wrap"><i data-lucide="flag" class="icon"></i><select class="input" disabled><option>請選擇國家</option>${list.map(o => `<option>${_esc(o)}</option>`).join('')}</select></div>`;
      break;
    }
    case 'number':
      inputHtml = `<div class="input-wrap"><i data-lucide="hash" class="icon"></i><input class="input" type="number" placeholder="${ph}" disabled></div>`;
      break;
    case 'select':
      inputHtml = `<div class="input-wrap"><select class="input" disabled><option>請選擇</option>${opts.map(o => `<option>${_esc(o)}</option>`).join('')}</select></div>`;
      break;
    case 'radio':
      inputHtml = `<div class="selection-group">${opts.map(o => `<label class="selection-chip"><input type="radio" disabled> ${_esc(o)}</label>`).join('')}</div>`;
      break;
    case 'checkbox':
      inputHtml = `<div class="selection-group">${opts.map(o => `<label class="selection-chip"><input type="checkbox" disabled> ${_esc(o)}</label>`).join('')}</div>`;
      break;
    case 'multilist':
      inputHtml = `<div class="fb-multilist">${opts.map(o => `<label class="fb-multilist-item"><input type="checkbox" disabled> ${_esc(o)}</label>`).join('')}</div>`;
      break;
    case 'url':
      inputHtml = `<div class="input-wrap"><i data-lucide="link" class="icon"></i><input class="input" type="url" placeholder="${ph || 'https://example.com'}" value="${dv}" disabled></div>`;
      break;
    case 'datetime':
      inputHtml = `<div class="input-wrap"><i data-lucide="calendar" class="icon"></i><input class="input" type="datetime-local" disabled></div>`;
      break;
    case 'image':
      inputHtml = `<div class="form-file-drop" style="pointer-events:none"><i data-lucide="image" class="icon"></i><span>點擊或拖曳圖片至此</span></div>`;
      break;
    case 'file':
      inputHtml = `<div class="form-file-drop" style="pointer-events:none"><i data-lucide="upload" class="icon"></i><span>點擊或拖曳檔案至此</span></div>`;
      break;
    case 'html':
      inputHtml = `<div class="fb-html-block"><i data-lucide="code" class="icon"></i> 自訂 HTML 區塊(實際渲染時呈現)</div>`;
      break;
    case 'phone':
      inputHtml = `<div class="input-wrap"><i data-lucide="phone" class="icon"></i><input class="input" type="tel" placeholder="${ph || '0900-000-000'}" value="${dv}" disabled></div>`;
      break;
  }

  // Standard preview row: handle (top-level only) + content + hover delete
  return `
    <div class="${cls}" data-fb-uid="${f.uid}"${draggableAttr}>
      ${handleHtml}
      <div class="fb-preview-content">${labelHtml}${inputHtml}</div>
      ${deleteBtn}
    </div>
  `;
}

function _fbRenderProperties() {
  const panel = document.getElementById('fb-properties');
  if (!panel) return;
  const f = _fbFields.find(x => x.uid === _fbSelectedFieldUid);
  if (!f) {
    panel.hidden = true;
    panel.innerHTML = '';
    return;
  }
  panel.hidden = false;
  // Layout rows — show only a layout-type dropdown (no required / placeholder)
  if (typeof f.type === 'string' && f.type.startsWith('layout-')) {
    const layoutOpts = [
      ['layout-1-1',   '2 欄佈局 1:1'],
      ['layout-2-1',   '2 欄佈局 2:1'],
      ['layout-1-2',   '2 欄佈局 1:2'],
      ['layout-1-1-1', '3 欄佈局 1:1:1'],
    ].map(([v, lbl]) => `<option value="${v}"${f.type === v ? ' selected' : ''}>${lbl}</option>`).join('');
    panel.innerHTML = `
      <div class="fb-prop-head">
        <div class="form-section-title fb-prop-title">分欄佈局</div>
      </div>
      <div class="form-section">
        <div class="form-field">
          <label class="form-field-label">佈局類型</label>
          <div class="input-wrap">
            <select class="input" data-fb-layout-change>${layoutOpts}</select>
          </div>
        </div>
      </div>
    `;
    return;
  }
  const supportsPlaceholder = FB_TYPES_WITH_PLACEHOLDER.has(f.type);
  const supportsDefault = FB_TYPES_WITH_DEFAULT.has(f.type);
  const supportsOptions = FB_TYPES_WITH_OPTIONS.has(f.type);
  const isDivider = f.type === 'divider';
  const isSectionTitle = f.type === 'section-title';
  const isAutofill = f.type === 'autofill';
  const isEmpSelect = f.type === 'employee-select';
  const isTextOnly = isDivider || isSectionTitle;
  const labelText = isDivider ? '段落標題' : isSectionTitle ? '分類標題' : '欄位標籤';

  panel.innerHTML = `
    <div class="fb-prop-head">
      <div class="form-section-title fb-prop-title">${FB_FIELD_TYPE_LABELS[f.type]}</div>
    </div>
    <div class="form-section">
      <div class="form-field">
        <label class="form-field-label">${labelText}</label>
        <div class="input-wrap"><input class="input" data-fb-prop="label" value="${_esc(f.label)}"></div>
      </div>
      ${!isTextOnly ? `
        <div class="form-field">
          <label class="fb-prop-toggle">
            <input type="checkbox" data-fb-prop="required" ${f.required ? 'checked' : ''}>
            <span>必填欄位</span>
          </label>
        </div>
      ` : ''}
      ${supportsPlaceholder ? `
        <div class="form-field">
          <label class="form-field-label">提示文字 (placeholder)</label>
          <div class="input-wrap"><input class="input" data-fb-prop="placeholder" value="${_esc(f.placeholder)}" placeholder="例:請輸入..."></div>
        </div>
      ` : ''}
      ${supportsDefault ? `
        <div class="form-field">
          <label class="form-field-label">預設值</label>
          <div class="input-wrap"><input class="input" data-fb-prop="defaultValue" value="${_esc(f.defaultValue)}"></div>
        </div>
      ` : ''}
      ${supportsOptions ? `
        <div class="form-field">
          <label class="form-field-label">選項(每行一個)</label>
          <textarea class="form-textarea" data-fb-prop="options" rows="5" placeholder="選項 1&#10;選項 2">${_esc(f.options)}</textarea>
        </div>
      ` : ''}
      ${isAutofill ? `
        <div class="form-field">
          <label class="form-field-label">API 來源</label>
          <div class="input-wrap"><input class="input" data-fb-prop="source" value="${_esc(f.source || '')}" placeholder="例:/api/user/profile.employeeId"></div>
          <div class="p-mini muted" style="margin-top:var(--space-2xs)">填入 API 端點或物件路徑,送出表單時會自動帶入此欄位值。</div>
        </div>
      ` : ''}
      ${isEmpSelect ? `
        <div class="form-field">
          <label class="form-field-label">可選範圍</label>
          <div class="input-wrap">
            <select class="input" data-fb-prop="scope">
              <option value="all"${f.scope === 'all' ? ' selected' : ''}>全部員工</option>
              <option value="dept"${f.scope === 'dept' ? ' selected' : ''}>部門員工</option>
            </select>
          </div>
        </div>
      ` : ''}
    </div>
  `;
  if (window.lucide?.createIcons) lucide.createIcons();
}

// Role key → human-readable label. Used by both the preview and the modal.
const _FB_STAGE_ROLE_LABELS = {
  manager:     '直屬主管',
  'dept-head': '部門主管',
  hr:          'HR 部門',
  finance:     '財務部門',
  ceo:         '總經理',
  relative:    '等級主管',  // runtime resolves with relativeLevel offset
  applicant:   '申請者',
};

function _fbStageDisplay(s) {
  if (s.role === 'custom') {
    const p = (typeof ORG_DATA !== 'undefined' ? ORG_DATA : []).find(r => r.id === s.personId);
    return p ? p.nameZh : '自訂人員(未選擇)';
  }
  if (s.role === 'relative') {
    const n = Number(s.relativeLevel) || 1;
    return `申請者等級 +${n} 以上主管`;
  }
  return _FB_STAGE_ROLE_LABELS[s.role] || '未設定角色';
}


function _fbAddStage(role = '') {
  // Legacy helper — kept for the seed defaults. Pushes an approver-type node.
  _fbStages.push({ uid: ++_fbFieldUid, type: 'approver', role, personId: '' });
  _fbRenderStages();
}

// Right-column read-only preview. Uses a vertical stepper layout — each
// stage is a numbered circle + label; a trailing "完成" node closes the
// chain. Condition stages fold their branched nodes inline so the preview
// shows e.g. "條件分支 - 時數 ≥ 40, 審核 部門主管".
function _fbRenderStages() {
  const host = document.getElementById('fb-stages-preview');
  if (!host) return;
  if (_fbStages.length === 0) {
    host.innerHTML = '<div class="fb-preview-step-empty">尚未設定審核關卡</div>';
    return;
  }

  const branchTitle = (b) => {
    if (b.type === 'approver') return `審核 ${_fbStageDisplay(b)}`;
    if (b.type === 'notify')   return `知會 ${_fbStageDisplay(b)}`;
    if (b.type === 'parallel') return `會簽 ${_fbStageDisplay({ role: b.role })}`;
    return _fbStageDisplay(b);
  };

  const items = [];
  const consumed = new Set();
  for (const s of _fbStages) {
    if (consumed.has(s.uid)) continue;
    if (s.parentCondition) continue;  // rendered inline under its condition
    if (s.type === 'condition') {
      const branched = _fbStages.filter(x => x.parentCondition === s.uid);
      branched.forEach(b => consumed.add(b.uid));
      const parts = [_fbConditionDisplay(s), ...branched.map(branchTitle)];
      items.push({ label: '條件分支', title: parts.join(', ') });
    } else {
      const meta = _FB_FLOW_TYPE_META[s.type] || _FB_FLOW_TYPE_META.approver;
      const title = s.type === 'parallel'
        ? _fbStageDisplay({ role: s.role })
        : _fbStageDisplay({ role: s.role, personId: s.personId });
      items.push({ label: meta.label, title });
    }
    consumed.add(s.uid);
  }

  const stagesHtml = items.map((it, i) => `
    <div class="fb-preview-step">
      <div class="fb-preview-step-marker">${i + 1}</div>
      <div class="fb-preview-step-body">
        <div class="fb-preview-step-label">${it.label}</div>
        <div class="fb-preview-step-role">${it.title}</div>
      </div>
    </div>
  `).join('');
  const endStep = `
    <div class="fb-preview-step fb-preview-step-end">
      <div class="fb-preview-step-marker"><i data-lucide="check" class="icon"></i></div>
      <div class="fb-preview-step-body">
        <div class="fb-preview-step-label">完成</div>
        <div class="fb-preview-step-role">流程結束</div>
      </div>
    </div>
  `;
  host.innerHTML = stagesHtml + endStep;
  if (window.lucide?.createIcons) lucide.createIcons();
}

// ---- Approval flow editor modal — node-based canvas ----
//
// Node schema:
//   approver:  { uid, type: 'approver',  role, personId }
//   condition: { uid, type: 'condition', expression }
//   parallel:  { uid, type: 'parallel',  role, rule: 'all'|'majority' }
//   notify:    { uid, type: 'notify',    role, personId }
//
// _fbStages stores the production data; the modal works on a deep-copied
// _fbStagesDraft so cancel can discard changes.

const _FB_FLOW_TYPE_META = {
  approver:  { label: '審核',     icon: 'user-check' },
  condition: { label: '條件分支', icon: 'git-branch' },
  parallel:  { label: '會簽',     icon: 'users' },
  notify:    { label: '知會',     icon: 'bell' },
};

let _fbStagesDraft = null;
let _fbFlowSelectedUid = null;

// 流程結束後是否自動產生新申請單(常見的鏈式 OA 流程,例如出差核准後自動產生報銷單)
let _fbAutoNext = { enabled: false, formName: '' };
let _fbAutoNextDraft = null;

// Form options for the auto-next dropdown — kept in sync with the forms list mock data
const _FB_AUTO_NEXT_FORMS = [
  '請假申請單', '加班核准申請單', '加班申請單',
];

function _fbCreateNode(type) {
  const base = { uid: ++_fbFieldUid, type };
  if (type === 'approver') return { ...base, role: '', personId: '', relativeLevel: 1 };
  if (type === 'condition') return { ...base, condField: 'hours', condOp: '>=', condValue: '' };
  if (type === 'parallel') return { ...base, role: '', rule: 'all' };
  if (type === 'notify') return { ...base, role: '', personId: '', relativeLevel: 1 };
  return base;
}

// Condition node label maps (時數 / 金額 / 職等 + operator symbols)
const _FB_COND_FIELD_LABEL = { hours: '時數', amount: '金額', level: '職等' };
const _FB_COND_OP_LABEL = { '>=': '≥', '<=': '≤', '==': '=', '>': '>', '<': '<' };

function _fbConditionDisplay(n) {
  if (n.condField === 'level') {
    const levels = Array.isArray(n.condLevels) ? [...n.condLevels].sort((a, b) => a - b) : [];
    if (!levels.length) return '未設定條件';
    return `職等 ${levels.join('、')}`;
  }
  const field = _FB_COND_FIELD_LABEL[n.condField];
  const op = _FB_COND_OP_LABEL[n.condOp];
  const value = n.condValue;
  if (!field || !op || value === '' || value == null) return '未設定條件';
  return `${field} ${op} ${value}`;
}

// Build the display title used inside a node (matches preview)
function _fbFlowNodeTitle(n) {
  if (n.type === 'condition') return _fbConditionDisplay(n);
  if (n.type === 'parallel')  return _fbStageDisplay({ role: n.role });
  return _fbStageDisplay({ role: n.role, personId: n.personId });
}

function _fbRenderFlowCanvas() {
  const canvas = document.getElementById('fb-flow-canvas');
  if (!canvas || !_fbStagesDraft) return;
  const parts = [];

  // Implicit start node
  parts.push(`
    <div class="fb-flow-node fb-flow-node-start">
      <div class="fb-flow-node-icon"><i data-lucide="play" class="icon"></i></div>
      <div class="fb-flow-node-body"><div class="fb-flow-node-title">申請送出</div></div>
    </div>
  `);

  const connector = (insertAt) => `
    <div class="fb-flow-connector">
      <button type="button" class="fb-flow-connector-add" data-fb-flow-insert-at="${insertAt}" aria-label="在此插入節點">
        <i data-lucide="plus" class="icon"></i>
      </button>
    </div>
  `;

  // Render a single node card (used by both the linear flow and branch rows)
  const renderNode = (n) => {
    const meta = _FB_FLOW_TYPE_META[n.type] || _FB_FLOW_TYPE_META.approver;
    const sel = _fbFlowSelectedUid === n.uid ? ' is-selected' : '';
    return `
      <div class="fb-flow-node fb-flow-node-${n.type}${sel}" data-fb-flow-node="${n.uid}" draggable="true" tabindex="0" role="button" aria-label="${meta.label} 節點(Alt+↑/↓ 可調整順序)">
        <i data-lucide="grip-vertical" class="icon fb-flow-node-grip"></i>
        <div class="fb-flow-node-icon"><i data-lucide="${meta.icon}" class="icon"></i></div>
        <div class="fb-flow-node-body">
          <div class="fb-flow-node-label">${meta.label}</div>
          <div class="fb-flow-node-title">${_fbFlowNodeTitle(n)}</div>
        </div>
        <button type="button" class="icon-button fb-flow-node-remove" data-fb-flow-remove="${n.uid}" aria-label="移除節點">
          <i data-lucide="x" class="icon"></i>
        </button>
      </div>
    `;
  };

  // Walk stages; consecutive condition+branched groups are gathered into a
  // single block. Each condition + its parentCondition-linked nodes become
  // one pocket, and multiple pockets sit side by side to the right of the
  // shared spine ("橫向排列" 多分支).
  let idx = 0;
  while (idx < _fbStagesDraft.length) {
    const n = _fbStagesDraft[idx];
    if (n.type === 'condition') {
      const groups = [];
      let j = idx;
      while (j < _fbStagesDraft.length && _fbStagesDraft[j].type === 'condition') {
        const cond = _fbStagesDraft[j];
        const branched = [];
        let k = j + 1;
        while (k < _fbStagesDraft.length && _fbStagesDraft[k].parentCondition === cond.uid) {
          branched.push({ node: _fbStagesDraft[k], at: k });
          k++;
        }
        if (branched.length === 0) break;
        groups.push({ cond, branched });
        j = k;
      }
      if (groups.length === 0) {
        parts.push(connector(idx));
        parts.push(renderNode(n));
        idx += 1;
        continue;
      }
      parts.push(connector(idx));
      const pocketsHtml = groups.map((g, i) => {
        const pocketBody = g.branched.map(b => `
          <div class="fb-flow-cond-pocket-link">
            <button type="button" class="fb-flow-connector-add" data-fb-flow-insert-at="${b.at}" data-fb-flow-insert-cond="${g.cond.uid}" aria-label="在此插入節點">
              <i data-lucide="plus" class="icon"></i>
            </button>
          </div>
          ${renderNode(b.node)}
        `).join('');
        return `<div class="fb-flow-cond-pocket" style="--pocket-index:${i}">${renderNode(g.cond)}${pocketBody}</div>`;
      }).join('');
      parts.push(`
        <div class="fb-flow-cond-block" style="--pocket-count:${groups.length}">
          <div class="fb-flow-cond-spine"></div>
          ${pocketsHtml}
        </div>
      `);
      idx = j;
    } else {
      parts.push(connector(idx));
      parts.push(renderNode(n));
      idx += 1;
    }
  }

  // Trailing connector + end node
  parts.push(connector(_fbStagesDraft.length));
  parts.push(`
    <div class="fb-flow-node fb-flow-node-end">
      <div class="fb-flow-node-icon"><i data-lucide="check" class="icon"></i></div>
      <div class="fb-flow-node-body"><div class="fb-flow-node-title">流程結束</div></div>
    </div>
  `);

  // Post-end: auto-generate next application form. When disabled → a small
  // stub button. When enabled → a real selectable/removable node (editable
  // via the right property panel). No connector — this is a separate config,
  // not part of the linear flow.
  const auto = _fbAutoNextDraft || { enabled: false, formName: '' };
  if (auto.enabled) {
    const isSel = _fbFlowSelectedUid === 'auto-next' ? ' is-selected' : '';
    parts.push(`
      <div class="fb-flow-connector"></div>
      <div class="fb-flow-node fb-flow-node-auto-next${isSel}" data-fb-auto-next-node tabindex="0">
        <div class="fb-flow-node-icon"><i data-lucide="repeat-2" class="icon"></i></div>
        <div class="fb-flow-node-body">
          <div class="fb-flow-node-label">自動產生新申請單</div>
          <div class="fb-flow-node-title">${auto.formName || '未選擇表單'}</div>
        </div>
        <button type="button" class="icon-button fb-flow-node-remove" data-fb-auto-next-remove aria-label="停用">
          <i data-lucide="x" class="icon"></i>
        </button>
      </div>
    `);
  } else {
    parts.push(`
      <div class="fb-flow-auto-next-spacer"></div>
      <button type="button" class="btn btn-outline btn-sm fb-flow-auto-next-stub" id="fb-auto-next-enable">
        <i data-lucide="repeat-2" class="icon"></i> 啟用自動產生新申請單
      </button>
    `);
  }

  canvas.innerHTML = `<div class="fb-flow-canvas-inner">${parts.join('')}</div>`;
  if (window.lucide?.createIcons) lucide.createIcons();
}

function _fbRenderFlowProps() {
  const host = document.getElementById('fb-flow-props');
  if (!host || !_fbStagesDraft) return;

  // Special-case: auto-next node uses its own draft state, not _fbStagesDraft
  if (_fbFlowSelectedUid === 'auto-next' && _fbAutoNextDraft?.enabled) {
    const formOpts = _FB_AUTO_NEXT_FORMS
      .map(name => `<option value="${name}"${_fbAutoNextDraft.formName === name ? ' selected' : ''}>${name}</option>`)
      .join('');
    host.innerHTML = `
      <div class="form-section-title">自動產生新申請單</div>
      <div class="form-field">
        <label class="form-field-label">選擇要自動產生的表單</label>
        <div class="input-wrap">
          <select class="input" data-fb-auto-next-form>
            <option value="">— 未選擇 —</option>
            ${formOpts}
          </select>
        </div>
      </div>
      <div class="p-mini muted">流程結束後系統會自動產生此表單,並通知申請人填寫。</div>
    `;
    if (window.lucide?.createIcons) lucide.createIcons();
    return;
  }

  const node = _fbStagesDraft.find(n => n.uid === _fbFlowSelectedUid);
  if (!node) {
    host.innerHTML = '<div class="fb-flow-props-empty p-small muted">點選節點即可編輯設定</div>';
    return;
  }
  const meta = _FB_FLOW_TYPE_META[node.type] || {};
  const roleOpts = (sel) => `
    <option value="">選擇角色</option>
    <option value="manager"${sel === 'manager' ? ' selected' : ''}>直屬主管</option>
    <option value="dept-head"${sel === 'dept-head' ? ' selected' : ''}>部門主管</option>
    <option value="relative"${sel === 'relative' ? ' selected' : ''}>申請者等級 +N 以上主管</option>
    <option value="hr"${sel === 'hr' ? ' selected' : ''}>HR 部門</option>
    <option value="finance"${sel === 'finance' ? ' selected' : ''}>財務部門</option>
    <option value="ceo"${sel === 'ceo' ? ' selected' : ''}>總經理</option>
    <option value="applicant"${sel === 'applicant' ? ' selected' : ''}>申請者本人</option>
    <option value="custom"${sel === 'custom' ? ' selected' : ''}>自訂人員</option>
  `;
  const managers = (typeof ORG_DATA !== 'undefined' ? ORG_DATA : []).filter(r => r.isManager);
  const personOpts = (sel) => `<option value="">選擇成員</option>${managers.map(m => `<option value="${m.id}"${sel === m.id ? ' selected' : ''}>${m.nameZh}(${m.dept})</option>`).join('')}`;
  // Re-usable block: relative-level number input (shown when role === 'relative')
  const relativeLevelBlock = (n) => `
    <div class="form-field">
      <label class="form-field-label">等級 (+N)</label>
      <div class="input-wrap"><input class="input" type="number" min="1" max="10" data-fb-flow-prop="relativeLevel" value="${n.relativeLevel ?? 1}" placeholder="例:2"></div>
      <div class="p-mini muted" style="margin-top:var(--space-2xs)">N=1 為直屬主管,N=2 為直屬主管的主管,以此類推。</div>
    </div>
  `;

  let bodyHtml = '';
  if (node.type === 'approver') {
    bodyHtml = `
      <div class="form-field">
        <label class="form-field-label">審核角色</label>
        <div class="input-wrap"><select class="input" data-fb-flow-prop="role">${roleOpts(node.role)}</select></div>
      </div>
      ${node.role === 'custom' ? `
        <div class="form-field">
          <label class="form-field-label">指派成員</label>
          <div class="input-wrap"><select class="input" data-fb-flow-prop="personId">${personOpts(node.personId)}</select></div>
        </div>
      ` : ''}
      ${node.role === 'relative' ? relativeLevelBlock(node) : ''}
    `;
  } else if (node.type === 'condition') {
    const fieldOpt = (v, lbl) => `<option value="${v}"${node.condField === v ? ' selected' : ''}>${lbl}</option>`;
    const opOpt = (v, lbl) => `<option value="${v}"${node.condOp === v ? ' selected' : ''}>${lbl}</option>`;
    const levels = Array.isArray(node.condLevels) ? node.condLevels : [];
    const levelChecks = [1, 2, 3, 4].map(lv => `
      <label class="fb-flow-level-option">
        <input type="checkbox" class="checkbox" data-fb-flow-level="${lv}" ${levels.includes(lv) ? 'checked' : ''}>
        職等 ${lv}
      </label>
    `).join('');
    const isLevel = node.condField === 'level';
    bodyHtml = `
      <div class="form-field">
        <label class="form-field-label">條件類型</label>
        <div class="input-wrap">
          <select class="input" data-fb-flow-prop="condField">
            ${fieldOpt('hours', '時數')}
            ${fieldOpt('amount', '金額')}
            ${fieldOpt('level',  '職等')}
          </select>
        </div>
      </div>
      ${isLevel ? `
        <div class="form-field">
          <label class="form-field-label">職等(可複選)</label>
          <div class="fb-flow-level-options">${levelChecks}</div>
        </div>
      ` : `
        <div class="form-field">
          <label class="form-field-label">運算子</label>
          <div class="input-wrap">
            <select class="input" data-fb-flow-prop="condOp">
              ${opOpt('>=', '≥ 大於等於')}
              ${opOpt('>',  '> 大於')}
              ${opOpt('==', '= 等於')}
              ${opOpt('<',  '< 小於')}
              ${opOpt('<=', '≤ 小於等於')}
            </select>
          </div>
        </div>
        <div class="form-field">
          <label class="form-field-label">數值</label>
          <div class="input-wrap"><input class="input" type="number" min="0" data-fb-flow-prop="condValue" value="${node.condValue ?? ''}" placeholder="例:40"></div>
        </div>
      `}
      <div class="p-mini muted">符合條件走分支,不符合走主線。</div>
    `;
  } else if (node.type === 'parallel') {
    bodyHtml = `
      <div class="form-field">
        <label class="form-field-label">會簽角色</label>
        <div class="input-wrap"><select class="input" data-fb-flow-prop="role">${roleOpts(node.role)}</select></div>
      </div>
      ${node.role === 'relative' ? relativeLevelBlock(node) : ''}
    `;
  } else if (node.type === 'notify') {
    bodyHtml = `
      <div class="form-field">
        <label class="form-field-label">通知對象</label>
        <div class="input-wrap"><select class="input" data-fb-flow-prop="role">${roleOpts(node.role)}</select></div>
      </div>
      ${node.role === 'custom' ? `
        <div class="form-field">
          <label class="form-field-label">指派成員</label>
          <div class="input-wrap"><select class="input" data-fb-flow-prop="personId">${personOpts(node.personId)}</select></div>
        </div>
      ` : ''}
      ${node.role === 'relative' ? relativeLevelBlock(node) : ''}
      <div class="p-mini muted">知會節點僅發送通知,不影響流程。</div>
    `;
  }

  // Node type select — change the node type in place, preserving uid AND
  // parentCondition (see onPropChange). Nodes inside a condition pocket can
  // only be 審核 / 知會 / 會簽; nested condition branches are not supported
  // because they'd require a second-level pocket layout.
  const inPocket = node.parentCondition != null;
  const typeOptions = inPocket
    ? ['approver', 'notify', 'parallel']
    : ['approver', 'condition', 'parallel', 'notify'];
  const typeLabel = { approver: '審核', condition: '條件分支', parallel: '會簽', notify: '知會' };
  const typeSelect = `
    <div class="form-field">
      <label class="form-field-label">節點類型</label>
      <div class="input-wrap">
        <select class="input" data-fb-flow-prop-type>
          ${typeOptions.map(v => `<option value="${v}"${node.type === v ? ' selected' : ''}>${typeLabel[v]}</option>`).join('')}
        </select>
      </div>
    </div>
  `;

  host.innerHTML = `
    <div class="form-section-title">${meta.label || ''} 設定</div>
    ${typeSelect}
    ${bodyHtml}
  `;
  if (window.lucide?.createIcons) lucide.createIcons();
}

function _fbRenderFlow() {
  _fbRenderFlowCanvas();
  _fbRenderFlowProps();
}

function openFbStagesModal() {
  const modal = document.getElementById('fb-stages-modal');
  if (!modal) return;
  // Migrate legacy shape (no `type` field) to the new schema
  _fbStagesDraft = _fbStages.map(s => ({
    ...s,
    type: s.type || 'approver',
  }));
  _fbAutoNextDraft = { ..._fbAutoNext };
  _fbFlowSelectedUid = null;
  _fbRenderFlow();
  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  if (window.lucide?.createIcons) lucide.createIcons();
}

function closeFbStagesModal() {
  const modal = document.getElementById('fb-stages-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
  _fbStagesDraft = null;
  _fbAutoNextDraft = null;
  _fbFlowSelectedUid = null;
}

function _saveFbStages() {
  if (_fbStagesDraft) _fbStages = _fbStagesDraft.map(s => ({ ...s }));
  if (_fbAutoNextDraft) _fbAutoNext = { ..._fbAutoNextDraft };
  closeFbStagesModal();
  _fbRenderStages();
}

function initFbStagesModal() {
  const modal = document.getElementById('fb-stages-modal');
  if (!modal) return;
  document.getElementById('fb-stages-modal-close')?.addEventListener('click', closeFbStagesModal);
  document.getElementById('fb-stages-modal-cancel')?.addEventListener('click', closeFbStagesModal);
  document.getElementById('fb-stages-modal-save')?.addEventListener('click', _saveFbStages);
  modal.addEventListener('click', (e) => { if (e.target === modal) closeFbStagesModal(); });

  // Palette buttons — append a new node of the chosen type.
  // Adding a 條件分支 automatically pairs it with an approver below (the
  // node displayed in the side pocket), since a condition without a target
  // action makes no sense visually.
  modal.querySelectorAll('[data-fb-flow-add]').forEach(btn => {
    btn.addEventListener('click', () => {
      if (!_fbStagesDraft) return;
      const type = btn.dataset.fbFlowAdd;
      const node = _fbCreateNode(type);
      _fbStagesDraft.push(node);
      if (type === 'condition') {
        const paired = _fbCreateNode('approver');
        paired.parentCondition = node.uid;
        _fbStagesDraft.push(paired);
      }
      _fbFlowSelectedUid = node.uid;
      _fbRenderFlow();
    });
  });

  const canvas = document.getElementById('fb-flow-canvas');
  canvas?.addEventListener('click', (e) => {
    if (!_fbStagesDraft) return;
    // Remove node — if this was a branched (parentCondition set) and the
    // pocket has no other nodes belonging to the same condition, drop the
    // condition too. Also if the removed node is a condition, cascade-
    // remove all of its branched nodes.
    const rm = e.target.closest('[data-fb-flow-remove]');
    if (rm) {
      const uid = +rm.dataset.fbFlowRemove;
      const i = _fbStagesDraft.findIndex(x => x.uid === uid);
      if (i >= 0) {
        const target = _fbStagesDraft[i];
        _fbStagesDraft.splice(i, 1);
        if (target.parentCondition) {
          const stillHasBranched = _fbStagesDraft.some(s => s.parentCondition === target.parentCondition);
          if (!stillHasBranched) {
            const condIdx = _fbStagesDraft.findIndex(s => s.uid === target.parentCondition);
            if (condIdx >= 0) _fbStagesDraft.splice(condIdx, 1);
          }
        } else if (target.type === 'condition') {
          _fbStagesDraft = _fbStagesDraft.filter(s => s.parentCondition !== target.uid);
        }
      }
      if (_fbFlowSelectedUid === uid) _fbFlowSelectedUid = null;
      _fbRenderFlow();
      e.stopPropagation();
      return;
    }
    // Insert at index. When the + was inside a condition pocket the button
    // carries data-fb-flow-insert-cond=<uid>; the new node inherits that
    // parentCondition so it stays inside the pocket. Always insert an approver
    // by default — users change the node type via the right-side property panel.
    const ins = e.target.closest('[data-fb-flow-insert-at]');
    if (ins) {
      const insertIdx = +ins.dataset.fbFlowInsertAt;
      const parentCondRaw = ins.dataset.fbFlowInsertCond;
      const node = _fbCreateNode('approver');
      if (parentCondRaw) node.parentCondition = +parentCondRaw;
      _fbStagesDraft.splice(insertIdx, 0, node);
      _fbFlowSelectedUid = node.uid;
      _fbRenderFlow();
      return;
    }
    // Select node
    const nodeEl = e.target.closest('[data-fb-flow-node]');
    if (nodeEl) {
      _fbFlowSelectedUid = +nodeEl.dataset.fbFlowNode;
      _fbRenderFlow();
      return;
    }
    // Auto-next: enable stub button → turn on + auto-select the new node
    if (e.target.closest('#fb-auto-next-enable')) {
      _fbAutoNextDraft = { enabled: true, formName: _fbAutoNextDraft?.formName || '' };
      _fbFlowSelectedUid = 'auto-next';
      _fbRenderFlow();
      return;
    }
    // Auto-next: remove (X) button on the node → disable, clear selection
    if (e.target.closest('[data-fb-auto-next-remove]')) {
      _fbAutoNextDraft = { enabled: false, formName: '' };
      if (_fbFlowSelectedUid === 'auto-next') _fbFlowSelectedUid = null;
      _fbRenderFlow();
      e.stopPropagation();
      return;
    }
    // Auto-next: click the node body → select it (opens property panel)
    if (e.target.closest('[data-fb-auto-next-node]')) {
      _fbFlowSelectedUid = 'auto-next';
      _fbRenderFlow();
      return;
    }
  });

  // Property panel — sync edits back to the draft node
  const props = document.getElementById('fb-flow-props');
  const onPropChange = (e) => {
    // Auto-next form dropdown — its own draft state
    const autoForm = e.target.closest('[data-fb-auto-next-form]');
    if (autoForm) {
      if (_fbAutoNextDraft) _fbAutoNextDraft.formName = autoForm.value;
      _fbRenderFlow();
      return;
    }

    if (!_fbStagesDraft) return;

    // Type change — replace the node in place with a fresh one of the new
    // type, preserving uid AND parentCondition (selection survives; pocket-
    // resident nodes stay inside the pocket). When switching TO condition,
    // also append a paired approver so the side pocket has a target action.
    const typeEl = e.target.closest('[data-fb-flow-prop-type]');
    if (typeEl) {
      const node = _fbStagesDraft.find(n => n.uid === _fbFlowSelectedUid);
      if (!node || node.type === typeEl.value) return;
      const fresh = _fbCreateNode(typeEl.value);
      fresh.uid = node.uid;
      if (node.parentCondition != null) fresh.parentCondition = node.parentCondition;
      const i = _fbStagesDraft.indexOf(node);
      _fbStagesDraft[i] = fresh;
      if (typeEl.value === 'condition') {
        const hasPaired = _fbStagesDraft.some(s => s.parentCondition === fresh.uid);
        if (!hasPaired) {
          const paired = _fbCreateNode('approver');
          paired.parentCondition = fresh.uid;
          _fbStagesDraft.splice(i + 1, 0, paired);
        }
      }
      _fbRenderFlow();
      return;
    }

    // 職等 multi-select checkboxes — toggle in node.condLevels array
    const levelEl = e.target.closest('[data-fb-flow-level]');
    if (levelEl) {
      const node = _fbStagesDraft.find(n => n.uid === _fbFlowSelectedUid);
      if (!node) return;
      const lv = Number(levelEl.dataset.fbFlowLevel);
      const set = new Set(Array.isArray(node.condLevels) ? node.condLevels : []);
      if (levelEl.checked) set.add(lv); else set.delete(lv);
      node.condLevels = [...set].sort((a, b) => a - b);
      _fbRenderFlow();
      return;
    }

    const el = e.target.closest('[data-fb-flow-prop]');
    if (!el) return;
    const node = _fbStagesDraft.find(n => n.uid === _fbFlowSelectedUid);
    if (!node) return;
    const key = el.dataset.fbFlowProp;
    // Coerce numeric inputs (relativeLevel, condValue) to actual numbers
    if (el.type === 'number') node[key] = el.value === '' ? '' : Number(el.value);
    else node[key] = el.value;
    if (key === 'role' && node.role !== 'custom') node.personId = '';
    // Switching condField to 'level' — initialize condLevels if missing
    if (key === 'condField' && el.value === 'level' && !Array.isArray(node.condLevels)) {
      node.condLevels = [];
    }
    _fbRenderFlow();
  };
  props?.addEventListener('input', onPropChange);
  props?.addEventListener('change', onPropChange);

  // Drag-to-reorder on the canvas — see _initFbFlowDrag for details
  _initFbFlowDrag();
  // Drag empty canvas area to pan the view (works alongside scroll wheel)
  _initFbFlowPan();
}

// Click-and-drag panning on the canvas background. Only starts when the
// pointer goes down on the canvas itself (not on a node or button), so
// node-click / node-drag-reorder still work.
function _initFbFlowPan() {
  const canvas = document.getElementById('fb-flow-canvas');
  if (!canvas || canvas._panBound) return;
  canvas._panBound = true;
  let panning = false;
  let startX = 0, startY = 0;
  let baseLeft = 0, baseTop = 0;
  canvas.addEventListener('mousedown', (e) => {
    // Pan only when clicking on canvas background / inner wrapper — never on
    // an actual node or interactive element so selection / drag-to-reorder
    // still work.
    const onBg = e.target === canvas ||
                 e.target.classList?.contains('fb-flow-canvas-inner');
    if (!onBg) return;
    panning = true;
    startX = e.pageX;
    startY = e.pageY;
    baseLeft = canvas.scrollLeft;
    baseTop = canvas.scrollTop;
    canvas.classList.add('is-panning');
    e.preventDefault();
  });
  const onMove = (e) => {
    if (!panning) return;
    canvas.scrollLeft = baseLeft - (e.pageX - startX);
    canvas.scrollTop = baseTop - (e.pageY - startY);
  };
  const onUp = () => {
    if (!panning) return;
    panning = false;
    canvas.classList.remove('is-panning');
  };
  document.addEventListener('mousemove', onMove);
  document.addEventListener('mouseup', onUp);
}

// HTML5 drag-and-drop reorder. Nodes are draggable; the connectors between
// nodes are the drop targets (each represents an insertion index).
function _initFbFlowDrag() {
  const canvas = document.getElementById('fb-flow-canvas');
  if (!canvas) return;
  let draggedUid = null;

  canvas.addEventListener('dragstart', (e) => {
    const nodeEl = e.target.closest('[data-fb-flow-node]');
    if (!nodeEl) return;
    draggedUid = +nodeEl.dataset.fbFlowNode;
    nodeEl.classList.add('is-dragging');
    e.dataTransfer.effectAllowed = 'move';
    // Firefox requires data to be set for the drag to fire
    e.dataTransfer.setData('text/plain', String(draggedUid));
  });

  canvas.addEventListener('dragend', () => {
    canvas.querySelectorAll('.is-dragging').forEach(el => el.classList.remove('is-dragging'));
    canvas.querySelectorAll('.is-drop-target').forEach(el => el.classList.remove('is-drop-target'));
    draggedUid = null;
  });

  canvas.addEventListener('dragover', (e) => {
    const conn = e.target.closest('.fb-flow-connector');
    if (!conn || draggedUid == null) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    // Highlight only the target connector
    canvas.querySelectorAll('.is-drop-target').forEach(el => el.classList.remove('is-drop-target'));
    conn.classList.add('is-drop-target');
  });

  canvas.addEventListener('dragleave', (e) => {
    const conn = e.target.closest('.fb-flow-connector');
    if (conn && !conn.contains(e.relatedTarget)) conn.classList.remove('is-drop-target');
  });

  canvas.addEventListener('drop', (e) => {
    const conn = e.target.closest('.fb-flow-connector');
    if (!conn || draggedUid == null || !_fbStagesDraft) return;
    e.preventDefault();
    const btn = conn.querySelector('[data-fb-flow-insert-at]');
    if (!btn) return;
    const toIdx = +btn.dataset.fbFlowInsertAt;
    const fromIdx = _fbStagesDraft.findIndex(n => n.uid === draggedUid);
    // No-op if dropping back at the same position
    if (fromIdx < 0 || toIdx === fromIdx || toIdx === fromIdx + 1) {
      draggedUid = null;
      _fbRenderFlow();
      return;
    }
    const [moved] = _fbStagesDraft.splice(fromIdx, 1);
    const adjusted = toIdx > fromIdx ? toIdx - 1 : toIdx;
    _fbStagesDraft.splice(adjusted, 0, moved);
    draggedUid = null;
    _fbRenderFlow();
  });

  // Keyboard reorder — Alt+↑/↓ moves the focused node one slot, Enter selects
  canvas.addEventListener('keydown', (e) => {
    const nodeEl = e.target.closest('[data-fb-flow-node]');
    if (!nodeEl || !_fbStagesDraft) return;
    const uid = +nodeEl.dataset.fbFlowNode;
    const i = _fbStagesDraft.findIndex(n => n.uid === uid);
    if (i < 0) return;
    if (e.altKey && e.key === 'ArrowUp' && i > 0) {
      e.preventDefault();
      [_fbStagesDraft[i - 1], _fbStagesDraft[i]] = [_fbStagesDraft[i], _fbStagesDraft[i - 1]];
      _fbRenderFlow();
      // Restore focus to the moved node after render
      requestAnimationFrame(() => document.querySelector(`[data-fb-flow-node="${uid}"]`)?.focus());
    } else if (e.altKey && e.key === 'ArrowDown' && i < _fbStagesDraft.length - 1) {
      e.preventDefault();
      [_fbStagesDraft[i + 1], _fbStagesDraft[i]] = [_fbStagesDraft[i], _fbStagesDraft[i + 1]];
      _fbRenderFlow();
      requestAnimationFrame(() => document.querySelector(`[data-fb-flow-node="${uid}"]`)?.focus());
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      _fbFlowSelectedUid = uid;
      _fbRenderFlow();
    } else if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault();
      _fbStagesDraft = _fbStagesDraft.filter(n => n.uid !== uid);
      if (_fbFlowSelectedUid === uid) _fbFlowSelectedUid = null;
      _fbRenderFlow();
    }
  });
}

// Sync the page title + emoji + breadcrumb to the form name in _fbBasic
function _fbSyncTitle() {
  const name = (_fbBasic.name || '').trim();
  const titleEl = document.getElementById('fb-page-title');
  const crumbEl = document.getElementById('fb-breadcrumb-current');
  const emojiEl = document.getElementById('fb-title-emoji');
  const display = name || '新增表單';
  if (titleEl) titleEl.textContent = display;
  if (crumbEl) crumbEl.textContent = display;
  if (emojiEl) emojiEl.textContent = _fbBasic.icon || '📝';
}

// ---------- Basic info modal ----------
function openFormBasicModal(mode = 'create') {
  const modal = document.getElementById('form-basic-modal');
  if (!modal) return;
  document.getElementById('form-basic-title').textContent = mode === 'edit' ? '編輯表單基本資料' : '新增表單';
  document.getElementById('form-basic-name').value = _fbBasic.name || '';
  document.getElementById('form-basic-category').value = _fbBasic.category || '';
  document.getElementById('form-basic-desc').value = _fbBasic.desc || '';
  document.querySelectorAll('#form-basic-icon-picker .fb-icon-option').forEach(b => {
    b.classList.toggle('selected', b.dataset.fbIcon === (_fbBasic.icon || '📝'));
  });
  document.getElementById('form-basic-submit').textContent = mode === 'edit' ? '儲存變更' : '下一步';
  document.getElementById('form-basic-submit').dataset.mode = mode;
  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  if (window.lucide?.createIcons) lucide.createIcons();
  setTimeout(() => document.getElementById('form-basic-name')?.focus(), 50);
}

function closeFormBasicModal() {
  const modal = document.getElementById('form-basic-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
}

function _showFormsView(view) {
  // view = 'list' | 'builder'
  document.querySelectorAll('[data-forms-view]').forEach(el => {
    el.hidden = el.dataset.formsView !== view;
  });
  // Lock body scroll while the fullscreen builder is open
  document.body.style.overflow = view === 'builder' ? 'hidden' : '';
  // Scroll workspace-main back to top so user sees the new view from the start
  document.querySelector('.workspace-main')?.scrollTo?.({ top: 0 });
  if (window.lucide?.createIcons) lucide.createIcons();
}

// Click 新增表單 in list → start by opening the basic-info modal
function showFormBuilder() {
  // Reset everything for a brand-new form
  _fbBasic = { name: '', category: '', icon: '📝', desc: '' };
  _fbFields = [];
  _fbStages = [];
  _fbFieldUid = 0;
  _fbSelectedFieldUid = null;
  openFormBasicModal('create');
}

// Called after the basic-info modal is submitted in 'create' mode
function _fbEnterBuilder() {
  // Seed default 3-stage approval flow per spec: 總經理 / 部門主管 / 直屬主管
  if (_fbStages.length === 0) {
    _fbAddStage('ceo');
    _fbAddStage('dept-head');
    _fbAddStage('manager');
  }
  _fbRenderPreview();
  _fbRenderProperties();
  _fbRenderStages();
  _fbSyncTitle();
  _showFormsView('builder');
}

// Forms list filters: 類別 dropdown / 狀態 dropdown / 搜尋表單名稱.
// Filter state is held in the closure and re-applied to the existing rows on
// every change — no virtualisation since the list is small.
function initFormListFilters() {
  const toolbar = document.getElementById('form-list-toolbar');
  const tbody = document.querySelector('.form-tbody');
  if (!toolbar || !tbody) return;

  const state = { cat: 'all', status: 'all', q: '' };
  const emptyEl = document.getElementById('form-list-empty');
  const tableCard = tbody.closest('.workspace-card');

  const apply = () => {
    const q = state.q.trim().toLowerCase();
    const isActive = state.cat !== 'all' || state.status !== 'all' || q.length > 0;
    let visible = 0;
    tbody.querySelectorAll('tr').forEach(row => {
      const cat = row.dataset.formCategory || '';
      const name = (row.dataset.formName || '').toLowerCase();
      const on = row.querySelector('.toggle')?.getAttribute('aria-checked') === 'true';

      const passCat = state.cat === 'all' || cat === state.cat;
      const passStatus = state.status === 'all'
        || (state.status === 'on' && on)
        || (state.status === 'off' && !on);
      const passSearch = !q || name.includes(q);

      const show = passCat && passStatus && passSearch;
      row.hidden = !show;
      if (show) visible++;
    });
    // When a filter / search is active and matches nothing, hide the whole
    // table card and show only the empty placeholder (no table headers /
    // borders peeking through).
    const showEmpty = isActive && visible === 0;
    if (emptyEl) emptyEl.hidden = !showEmpty;
    if (tableCard) tableCard.hidden = showEmpty;
  };

  // Re-apply when a row toggle is flipped so the 狀態 filter stays accurate
  tbody.addEventListener('click', (e) => {
    if (e.target.closest('.toggle')) {
      // Defer until the toggle's aria-checked has been updated by its own handler
      setTimeout(apply, 0);
    }
  });

  // Category dropdown
  const catDD = toolbar.querySelector('[data-form-cat-trigger]')?.closest('.dropdown');
  const catLabel = toolbar.querySelector('[data-form-cat-label]');
  const catTrigger = toolbar.querySelector('[data-form-cat-trigger]');
  catTrigger?.addEventListener('click', (e) => {
    e.stopPropagation();
    toggleDropdownExclusive(catDD);
  });
  catDD?.querySelectorAll('[data-form-cat]').forEach(item => {
    item.addEventListener('click', () => {
      const val = item.dataset.formCat;
      state.cat = val;
      catDD.querySelectorAll('[data-form-cat]').forEach(i => i.classList.remove('selected'));
      item.classList.add('selected');
      if (catLabel) catLabel.textContent = val === 'all' ? '類別' : val;
      catTrigger?.classList.toggle('is-filtered', val !== 'all');
      catDD.classList.remove('open');
      apply();
    });
  });

  // Status dropdown
  const statusDD = toolbar.querySelector('[data-form-status-trigger]')?.closest('.dropdown');
  const statusLabel = toolbar.querySelector('[data-form-status-label]');
  const statusTrigger = toolbar.querySelector('[data-form-status-trigger]');
  const STATUS_LABEL = { all: '狀態', on: '啟用', off: '未啟用' };
  statusTrigger?.addEventListener('click', (e) => {
    e.stopPropagation();
    toggleDropdownExclusive(statusDD);
  });
  statusDD?.querySelectorAll('[data-form-status]').forEach(item => {
    item.addEventListener('click', () => {
      const val = item.dataset.formStatus;
      state.status = val;
      statusDD.querySelectorAll('[data-form-status]').forEach(i => i.classList.remove('selected'));
      item.classList.add('selected');
      if (statusLabel) statusLabel.textContent = STATUS_LABEL[val] || '狀態';
      statusTrigger?.classList.toggle('is-filtered', val !== 'all');
      statusDD.classList.remove('open');
      apply();
    });
  });

  // Search input — debounce keystrokes so we don't spam the row scan
  const search = document.getElementById('form-list-search');
  let t;
  search?.addEventListener('input', () => {
    clearTimeout(t);
    t = setTimeout(() => { state.q = search.value; apply(); }, 120);
  });
}

function initFormBuilder() {
  const openBtn = document.getElementById('form-builder-open');
  if (!openBtn) return;

  openBtn.addEventListener('click', showFormBuilder);
  // Breadcrumb back + cancel both return to the list view
  document.querySelectorAll('[data-forms-back]').forEach(btn => {
    btn.addEventListener('click', () => _showFormsView('list'));
  });

  // ===== Forms list — row action menu (編輯 / 刪除) =====
  const formTbody = document.querySelector('.form-tbody');
  if (formTbody) {
    formTbody.addEventListener('click', (e) => {
      // Toggle the dropdown when the ellipsis is clicked
      const trigger = e.target.closest('.form-row-menu > .icon-button');
      if (trigger) {
        e.stopPropagation();
        toggleDropdownExclusive(trigger.closest('.dropdown'));
        return;
      }
      // Edit / Delete actions
      const action = e.target.closest('[data-form-action]');
      if (!action) return;
      e.stopPropagation();
      const row = action.closest('tr');
      const dd = action.closest('.dropdown');
      dd?.classList.remove('open');
      const op = action.dataset.formAction;
      const name = row.dataset.formName || '';
      const icon = row.dataset.formIcon || '📝';
      const category = row.dataset.formCategory || '';

      if (op === 'preview') {
        if (DYNAMIC_FORMS.has(name)) {
          // Seed the form so the modal renders the same fields the builder
          // shows, then open form-modal in preview mode (keeps the right-side
          // workflow panel intact).
          _fbBasic = { name, category, icon, desc: '' };
          _fbSeedFormByName(name);
          openFormModal(name, 'preview');
        } else {
          // Other forms still use the legacy modal (no dynamic schema yet)
          openFormModal(name);
        }
      } else if (op === 'edit') {
        // Skip the basic-info modal — go straight into the field-design page
        // with this form's name/icon/category prefilled.
        _fbBasic = { name, category, icon, desc: '' };
        _fbFields = [];
        _fbStages = [];
        _fbFieldUid = 0;
        _fbSelectedFieldUid = null;
        // Seed full fields + flow when the form has a PDF-backed spec
        if (DYNAMIC_FORMS.has(name)) _fbSeedFormByName(name);
        _fbEnterBuilder();
      } else if (op === 'delete') {
        confirmDialog({
          title: `確定刪除表單「${name}」?`,
          desc: '刪除後員工將無法再使用此表單,既有的歷史申請紀錄不受影響。',
          confirmText: '確定刪除',
          onConfirm: () => {
            row.remove();
            showToast?.({ title: `已刪除表單「${name}」`, variant: 'success' });
          }
        });
      }
    });
    // Close menus when clicking outside
    document.addEventListener('click', () => {
      formTbody.querySelectorAll('.form-row-menu.open').forEach(dd => dd.classList.remove('open'));
    });
  }

  // ===== Forms list — category / status / search filters =====
  initFormListFilters();

  // ===== Basic info modal =====
  const basicModal = document.getElementById('form-basic-modal');
  document.getElementById('form-basic-close')?.addEventListener('click', closeFormBasicModal);
  document.getElementById('form-basic-cancel')?.addEventListener('click', closeFormBasicModal);
  basicModal?.addEventListener('click', (e) => { if (e.target === basicModal) closeFormBasicModal(); });

  // Icon picker (inside basic modal) — toggle selected
  document.getElementById('form-basic-icon-picker')?.addEventListener('click', (e) => {
    const btn = e.target.closest('.fb-icon-option');
    if (!btn) return;
    document.querySelectorAll('#form-basic-icon-picker .fb-icon-option').forEach(b => b.classList.remove('selected'));
    btn.classList.add('selected');
  });

  // Submit handler — validates, writes to _fbBasic, then enters builder (create) or just updates (edit)
  document.getElementById('form-basic-submit')?.addEventListener('click', (e) => {
    const mode = e.currentTarget.dataset.mode || 'create';
    const name = document.getElementById('form-basic-name').value.trim();
    const category = document.getElementById('form-basic-category').value;
    if (!name) { showToast?.({ title: '請填入表單名稱', variant: 'warning' }); return; }
    if (!category) { showToast?.({ title: '請選擇表單分類', variant: 'warning' }); return; }
    _fbBasic.name = name;
    _fbBasic.category = category;
    _fbBasic.desc = document.getElementById('form-basic-desc').value.trim();
    _fbBasic.icon = document.querySelector('#form-basic-icon-picker .fb-icon-option.selected')?.dataset.fbIcon || '📝';
    closeFormBasicModal();
    if (mode === 'create') {
      _fbEnterBuilder();
    } else {
      _fbSyncTitle();
      showToast?.({ title: '基本資料已更新', variant: 'success' });
    }
  });

  // Edit icon next to title — reopen modal in 'edit' mode
  document.getElementById('fb-edit-basic')?.addEventListener('click', () => openFormBasicModal('edit'));

  // Live-preview modal — close handlers
  document.getElementById('fb-preview-modal-close')?.addEventListener('click', _fbCloseLivePreview);
  document.getElementById('fb-preview-modal-cancel')?.addEventListener('click', _fbCloseLivePreview);
  document.getElementById('fb-preview-modal')?.addEventListener('click', (e) => {
    if (e.target.id === 'fb-preview-modal') _fbCloseLivePreview();
  });

  // 儲存 / 預覽 — delegated so both desktop topbar + mobile palette buttons work
  document.addEventListener('click', (e) => {
    const action = e.target.closest('[data-fb-action]');
    if (!action) return;
    const which = action.dataset.fbAction;
    if (which === 'save') {
      if (!_fbBasic.name) { showToast?.({ title: '請先填寫基本資料', variant: 'warning' }); openFormBasicModal('edit'); return; }
      if (_fbFields.length === 0) { showToast?.({ title: '請至少新增 1 個欄位', variant: 'warning' }); return; }
      _showFormsView('list');
      showToast?.({
        title: `表單「${_fbBasic.name}」已儲存`,
        desc: `${_fbFields.length} 個欄位 · ${_fbStages.length} 關審核`,
        variant: 'success'
      });
    } else if (which === 'preview') {
      // Open the form-modal in preview mode — uses _fbFields for the form
      // body and keeps the right-side workflow panel visible.
      openFormModal(_fbBasic.name || '請假申請單', 'preview');
    }
  });

  // Step 2 — palette buttons → add field (auto-selects new one)
  document.querySelectorAll('[data-fb-add]').forEach(btn => {
    btn.addEventListener('click', () => _fbAddField(btn.dataset.fbAdd));
  });
  // Step 2 — layout buttons (4 ratio variants) → add column row to preview
  document.querySelectorAll('[data-fb-layout]').forEach(btn => {
    btn.addEventListener('click', () => _fbAddLayoutRow(btn.dataset.fbLayout));
  });

  // Step 2 — click a preview field to select it
  const fieldsList = document.getElementById('fb-fields-list');
  fieldsList?.addEventListener('click', (e) => {
    const field = e.target.closest('.fb-preview-field');
    if (!field) return;
    const uid = +field.dataset.fbUid;
    if (_fbSelectedFieldUid !== uid) {
      _fbSelectedFieldUid = uid;
      _fbRenderPreview();
      _fbRenderProperties();
    }
  });

  // Click anywhere inside the builder that's not a preview field or the
  // properties panel → clear the selection (panel hides). Scoped to the
  // builder so clicks on the topbar / palette don't fire this.
  const builder = document.querySelector('.fb-builder');
  builder?.addEventListener('click', (e) => {
    if (_fbSelectedFieldUid == null) return;
    if (e.target.closest('.fb-preview-field')) return;
    if (e.target.closest('.fb-properties')) return;
    if (e.target.closest('.fb-palette')) return;  // palette adds a field + auto-selects
    _fbSelectedFieldUid = null;
    _fbRenderPreview();
    _fbRenderProperties();
  });

  // Step 2 — drag & drop to reorder preview fields, AND drop palette
  // items into column slots. The dataTransfer payload distinguishes the
  // two cases:
  //   "field:<uid>"   — moving an existing top-level field (reorder)
  //   "palette:<type>" — dropping a new field type from the palette
  let _fbDragRow = null;

  // Palette items dragstart — set payload "palette:<type>"
  document.querySelectorAll('.fb-palette-item').forEach(btn => {
    btn.addEventListener('dragstart', (e) => {
      e.dataTransfer.setData('text/plain', `palette:${btn.dataset.fbAdd}`);
      e.dataTransfer.effectAllowed = 'copy';
    });
  });

  fieldsList?.addEventListener('dragstart', (e) => {
    const row = e.target.closest('.fb-preview-field');
    if (!row) return;
    // Don't start reorder drag from a child (inside a slot)
    if (row.classList.contains('is-child')) return;
    _fbDragRow = row;
    row.classList.add('is-dragging');
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', `field:${row.dataset.fbUid}`);
  });

  fieldsList?.addEventListener('dragover', (e) => {
    // 1) Highlight a column slot we're hovering — palette drops land there
    const slot = e.target.closest('.fb-cols-slot:not(.is-filled)');
    document.querySelectorAll('.fb-cols-slot.is-drop-target').forEach(s => {
      if (s !== slot) s.classList.remove('is-drop-target');
    });
    if (slot) {
      e.preventDefault();
      slot.classList.add('is-drop-target');
      return;
    }
    // 2) Reorder existing top-level fields by dragging
    if (_fbDragRow) {
      e.preventDefault();
      const target = e.target.closest('.fb-preview-field');
      if (!target || target === _fbDragRow || target.classList.contains('is-child')) return;
      const rect = target.getBoundingClientRect();
      const after = (e.clientY - rect.top) > rect.height / 2;
      if (after) target.parentNode.insertBefore(_fbDragRow, target.nextSibling);
      else target.parentNode.insertBefore(_fbDragRow, target);
      return;
    }
    // 3) Palette drag onto the preview area — allow drop anywhere
    e.preventDefault();
  });

  fieldsList?.addEventListener('drop', (e) => {
    const data = e.dataTransfer.getData('text/plain') || '';
    if (!data.startsWith('palette:')) return;
    e.preventDefault();
    const type = data.slice('palette:'.length);

    // Build the new field object (shared between slot / top-level drops)
    _fbFieldUid++;
    const f = {
      uid: _fbFieldUid,
      type,
      label: _fbDefaultLabel(type, _fbFields.length + 1),
      required: false,
      placeholder: '',
      defaultValue: '',
      options: '',
    };
    if (['select', 'radio', 'checkbox', 'multilist'].includes(type)) {
      f.options = '選項 1\n選項 2\n選項 3';
    } else if (type === 'country') {
      f.options = '台灣\n日本\n韓國\n美國';
    }

    // (a) palette → column slot (child field)
    const slot = e.target.closest('.fb-cols-slot:not(.is-filled)');
    if (slot) {
      f.parentRow = +slot.dataset.fbColsUid;
      f.slotIndex = +slot.dataset.fbColsSlot;
      _fbFields.push(f);
      slot.classList.remove('is-drop-target');
    } else {
      // (b) palette → top-level: insert at cursor position
      const target = e.target.closest('.fb-preview-field:not(.is-child)');
      if (target) {
        const rect = target.getBoundingClientRect();
        const after = (e.clientY - rect.top) > rect.height / 2;
        const targetUid = +target.dataset.fbUid;
        const targetIdx = _fbFields.findIndex(x => x.uid === targetUid);
        _fbFields.splice(targetIdx + (after ? 1 : 0), 0, f);
      } else {
        // empty area or below all fields — append at end
        _fbFields.push(f);
      }
    }
    _fbSelectedFieldUid = f.uid;
    _fbRenderPreview();
    _fbRenderProperties();
  });

  fieldsList?.addEventListener('dragleave', (e) => {
    const slot = e.target.closest('.fb-cols-slot');
    if (slot) slot.classList.remove('is-drop-target');
  });

  fieldsList?.addEventListener('dragend', () => {
    if (_fbDragRow) _fbDragRow.classList.remove('is-dragging');
    _fbDragRow = null;
    // Sync the in-memory order to the DOM order after a top-level reorder.
    // Children (inside slots) are intentionally not in topRows.
    const topRows = [...fieldsList.querySelectorAll(':scope > .fb-preview-field')]
      .map(el => +el.dataset.fbUid);
    const topLevel = _fbFields.filter(f => !f.parentRow);
    const children = _fbFields.filter(f => f.parentRow);
    topLevel.sort((a, b) => topRows.indexOf(a.uid) - topRows.indexOf(b.uid));
    _fbFields = [...topLevel, ...children];
    document.querySelectorAll('.fb-cols-slot.is-drop-target').forEach(s => s.classList.remove('is-drop-target'));
  });

  // Keyboard reorder for top-level preview fields — mirrors the flow-node
  // pattern: Alt+↑/↓ moves the focused field; Delete removes it.
  fieldsList?.addEventListener('keydown', (e) => {
    const fieldEl = e.target.closest('.fb-preview-field:not(.is-child)');
    if (!fieldEl) return;
    const uid = +fieldEl.dataset.fbUid;
    // Only reorder among top-level fields (parentRow == null)
    const topUids = _fbFields.filter(f => !f.parentRow).map(f => f.uid);
    const i = topUids.indexOf(uid);
    if (i < 0) return;
    if (e.altKey && e.key === 'ArrowUp' && i > 0) {
      e.preventDefault();
      const order = [...topUids];
      [order[i - 1], order[i]] = [order[i], order[i - 1]];
      const top = _fbFields.filter(f => !f.parentRow).sort((a, b) => order.indexOf(a.uid) - order.indexOf(b.uid));
      const children = _fbFields.filter(f => f.parentRow);
      _fbFields = [...top, ...children];
      _fbRenderPreview();
      requestAnimationFrame(() => document.querySelector(`.fb-preview-field[data-fb-uid="${uid}"]`)?.focus());
    } else if (e.altKey && e.key === 'ArrowDown' && i < topUids.length - 1) {
      e.preventDefault();
      const order = [...topUids];
      [order[i + 1], order[i]] = [order[i], order[i + 1]];
      const top = _fbFields.filter(f => !f.parentRow).sort((a, b) => order.indexOf(a.uid) - order.indexOf(b.uid));
      const children = _fbFields.filter(f => f.parentRow);
      _fbFields = [...top, ...children];
      _fbRenderPreview();
      requestAnimationFrame(() => document.querySelector(`.fb-preview-field[data-fb-uid="${uid}"]`)?.focus());
    } else if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault();
      _fbFields = _fbFields.filter(f => f.uid !== uid && f.parentRow !== uid);
      if (_fbSelectedFieldUid === uid) _fbSelectedFieldUid = null;
      _fbRenderPreview();
      _fbRenderProperties();
    }
  });

  // Hover-delete buttons on preview fields (top-level + children)
  fieldsList?.addEventListener('click', (e) => {
    const del = e.target.closest('[data-fb-delete]');
    if (!del) return;
    e.stopPropagation();  // don't trigger select on the field
    const uid = +del.dataset.fbDelete;
    // Also remove any children of a deleted column row
    _fbFields = _fbFields.filter(f => f.uid !== uid && f.parentRow !== uid);
    if (_fbSelectedFieldUid === uid) _fbSelectedFieldUid = null;
    _fbRenderPreview();
    _fbRenderProperties();
  });

  // Step 2 — properties panel handles label / placeholder / required / options edits.
  // For text inputs use 'input' to live-update; for checkbox use 'change'.
  const props = document.getElementById('fb-properties');
  props?.addEventListener('input', (e) => {
    const el = e.target.closest('[data-fb-prop]');
    if (!el || el.type === 'checkbox') return;
    const f = _fbFields.find(x => x.uid === _fbSelectedFieldUid);
    if (!f) return;
    const key = el.dataset.fbProp;
    f[key] = el.value;
    _fbRenderPreview();  // re-render preview only — properties keeps focus
  });
  props?.addEventListener('change', (e) => {
    const el = e.target.closest('[data-fb-prop]');
    if (el && el.type === 'checkbox') {
      const f = _fbFields.find(x => x.uid === _fbSelectedFieldUid);
      if (!f) return;
      f[el.dataset.fbProp] = el.checked;
      _fbRenderPreview();
      return;
    }
    // Layout type change — swap the row's type and drop children whose
    // slotIndex is out of range for the new column count.
    const layoutEl = e.target.closest('[data-fb-layout-change]');
    if (layoutEl) {
      const f = _fbFields.find(x => x.uid === _fbSelectedFieldUid);
      if (!f) return;
      f.type = layoutEl.value;
      const newCount = layoutEl.value.slice('layout-'.length).split('-').length;
      _fbFields = _fbFields.filter(c => !(c.parentRow === f.uid && c.slotIndex >= newCount));
      _fbRenderPreview();
      _fbRenderProperties();
    }
  });
  // ===== Approval flow (right column) =====
  // Right panel shows a preview; clicking "編輯審核流程" opens the editor modal.
  document.getElementById('fb-edit-stages')?.addEventListener('click', openFbStagesModal);
  initFbStagesModal();
}

// ---------- 管理員設定 ----------
let _adminSelectedEmp = null;
let _adminAddPermDraft = null;
const adminSelectedIds = new Set();

function _defaultAdminPerms() {
  // Sensible starting permissions for a brand-new admin: preview everything,
  // edit nothing. User can opt-in to edit per function before submitting.
  const def = {};
  _ADMIN_SECTIONS.forEach(g => g.items.forEach(it => { def[it.key] = 'view'; }));
  return def;
}

// Workspace sidebar sections (must match data-workspace-section keys in HTML).
// 概覽 and 操作紀錄 are omitted — they have no editable settings (概覽 is a
// dashboard, 操作紀錄 is a read-only log) and therefore aren't permission-gated.
const _ADMIN_SECTIONS = [
  { group: '人力管理', items: [
    { key: 'employees',    label: '員工管理',   icon: 'users' },
    { key: 'salary',       label: '員工薪資',   icon: 'dollar-sign' },
    { key: 'organization', label: '組織架構',   icon: 'network' },
  ]},
  { group: '制度設定', items: [
    { key: 'forms',         label: '表單設計',   icon: 'file-text' },
    { key: 'leave-policy',  label: '假勤制度',   icon: 'calendar-clock' },
  ]},
  { group: '系統設定', items: [
    { key: 'admins',     label: '管理員設定', icon: 'shield-check' },
  ]},
];

// Mock per-admin permissions. Values: 'none' | 'view' | 'edit'. Keys match
// the workspace sidebar's data-workspace-section attribute.
const _ADMIN_PERMS = {
  'IKL-2014-0021': { employees: 'edit', salary: 'edit', organization: 'edit', forms: 'edit', 'leave-policy': 'edit', admins: 'edit' },
  'IKL-2018-0205': { employees: 'edit', salary: 'edit', organization: 'view', forms: 'edit', 'leave-policy': 'edit', admins: 'view' },
  'IKL-2022-0188': { employees: 'view', salary: 'view', organization: 'view', forms: 'view', 'leave-policy': 'view', admins: 'none' },
  'IKL-2025-0312': { employees: 'edit', salary: 'none', organization: 'view', forms: 'none', 'leave-policy': 'view', admins: 'none' },
};

function _adminGetPerms(id) {
  // Default: view-only on all sections for unknown admins (e.g. newly added)
  if (!_ADMIN_PERMS[id]) {
    const def = {};
    _ADMIN_SECTIONS.forEach(g => g.items.forEach(it => { def[it.key] = 'view'; }));
    _ADMIN_PERMS[id] = def;
  }
  return _ADMIN_PERMS[id];
}

function _renderAdminPermSummary(adminId) {
  const row = document.querySelector(`tr[data-admin-id="${adminId}"]`);
  const span = row?.querySelector('[data-admin-perm-summary]');
  if (!span) return;
  const perms = _adminGetPerms(adminId);
  let view = 0, edit = 0;
  Object.values(perms).forEach(p => {
    if (p === 'view') view++;
    else if (p === 'edit') edit++;
  });
  span.textContent = `預覽 ${view} · 編輯 ${edit}`;
}

function _renderAllAdminSummaries() {
  document.querySelectorAll('#admin-tbody tr[data-admin-id]').forEach(row => {
    _renderAdminPermSummary(row.dataset.adminId);
  });
}

// ---- Admin permission edit modal ----

let _adminPermEditingId = null;
let _adminPermDraft = null;  // per-key staging copy so cancel discards

// Shared renderer: paints the per-function checkbox UI into `host` based on
// `draft`. Used by both the edit-perm modal and the new-admin modal.
function _renderAdminPermListInto(host, draft) {
  if (!host || !draft) return;
  host.innerHTML = _ADMIN_SECTIONS.map(group => `
    <div class="admin-perm-group">
      <div class="admin-perm-row admin-perm-row-head">
        <div class="admin-perm-group-label">${group.group}</div>
        <div>預覽</div>
        <div>編輯</div>
      </div>
      ${group.items.map(it => {
        const val = draft[it.key] || 'none';
        const viewChecked = val === 'view' || val === 'edit';
        const editChecked = val === 'edit';
        return `
          <div class="admin-perm-row" data-perm-key="${it.key}">
            <div class="admin-perm-row-name"><i data-lucide="${it.icon}" class="icon"></i>${it.label}</div>
            <label class="admin-perm-check">
              <input type="checkbox" class="checkbox" data-perm-mode="view" ${viewChecked ? 'checked' : ''} ${editChecked ? 'disabled' : ''}>
            </label>
            <label class="admin-perm-check">
              <input type="checkbox" class="checkbox" data-perm-mode="edit" ${editChecked ? 'checked' : ''}>
            </label>
          </div>
        `;
      }).join('')}
    </div>
  `).join('');
  if (window.lucide?.createIcons) lucide.createIcons();
}

// Wire change events on a perm-list container. `getDraft` is a thunk so the
// handler always reads the latest draft reference (drafts get re-assigned on
// modal open).
function _bindAdminPermListEvents(host, getDraft) {
  if (!host) return;
  host.addEventListener('change', (e) => {
    const input = e.target.closest('input[type="checkbox"]');
    if (!input) return;
    const draft = getDraft();
    if (!draft) return;
    const row = input.closest('[data-perm-key]');
    const key = row?.dataset.permKey;
    if (!key) return;
    const mode = input.dataset.permMode;
    if (mode === 'edit') {
      // Toggle on → edit (implies view, disables view).
      // Toggle off → downgrade to view (don't drop to none).
      draft[key] = input.checked ? 'edit' : 'view';
      _renderAdminPermListInto(host, draft);
    } else if (mode === 'view') {
      draft[key] = input.checked ? 'view' : 'none';
    }
  });
}

function openAdminPermModal(adminId) {
  const modal = document.getElementById('admin-perm-modal');
  if (!modal) return;
  _adminPermEditingId = adminId;
  // Shallow-copy current perms into a staging draft
  _adminPermDraft = { ..._adminGetPerms(adminId) };

  // Title = "{admin name} · 編輯權限"
  const row = document.querySelector(`tr[data-admin-id="${adminId}"]`);
  const name = row?.querySelector('.workspace-member .p-medium')?.textContent.trim() || '';
  const title = document.getElementById('admin-perm-modal-title');
  if (title) title.textContent = name ? `${name} · 編輯權限` : '編輯權限';

  _renderAdminPermListInto(document.getElementById('admin-perm-list'), _adminPermDraft);
  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
}

function closeAdminPermModal() {
  const modal = document.getElementById('admin-perm-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
  _adminPermEditingId = null;
  _adminPermDraft = null;
}

function _saveAdminPermChanges() {
  if (!_adminPermEditingId || !_adminPermDraft) return;
  _ADMIN_PERMS[_adminPermEditingId] = { ..._adminPermDraft };
  _renderAdminPermSummary(_adminPermEditingId);
  closeAdminPermModal();
  showToast?.({ title: '權限已更新', variant: 'success' });
}

function initAdminPermModal() {
  const modal = document.getElementById('admin-perm-modal');
  if (!modal) return;
  document.getElementById('admin-perm-close')?.addEventListener('click', closeAdminPermModal);
  document.getElementById('admin-perm-cancel')?.addEventListener('click', closeAdminPermModal);
  document.getElementById('admin-perm-save')?.addEventListener('click', _saveAdminPermChanges);
  modal.addEventListener('click', (e) => { if (e.target === modal) closeAdminPermModal(); });
  _bindAdminPermListEvents(document.getElementById('admin-perm-list'), () => _adminPermDraft);
}

function _adminSyncBulkBar() {
  const tbody = document.getElementById('admin-tbody');
  if (!tbody) return;
  const rows = [...tbody.querySelectorAll('tr')];
  // Sync each row's checkbox state to match selection set
  rows.forEach(r => {
    const id = r.dataset.adminId;
    const cb = r.querySelector('.admin-row-check');
    if (cb) cb.setAttribute('aria-checked', adminSelectedIds.has(id) ? 'true' : 'false');
  });
  // Master checkbox tri-state based on visible rows
  const all = document.getElementById('admin-select-all');
  if (all) {
    const ids = rows.map(r => r.dataset.adminId);
    const checked = ids.filter(id => adminSelectedIds.has(id)).length;
    if (checked === 0) all.setAttribute('aria-checked', 'false');
    else if (checked === ids.length) all.setAttribute('aria-checked', 'true');
    else all.setAttribute('aria-checked', 'mixed');
  }
  // Floating bulk action bar
  const bar = document.getElementById('admin-bulk-actionbar');
  const count = document.getElementById('admin-bulk-count');
  if (count) count.textContent = adminSelectedIds.size;
  if (bar) bar.hidden = adminSelectedIds.size === 0;
}

function _positionAdminEmpResults() {
  const input = document.getElementById('admin-emp-input');
  const list = document.getElementById('admin-emp-results');
  if (!input || !list) return;
  // Anchor the floating list to the .input-wrap (the visual input bounds)
  const wrap = input.closest('.input-wrap') || input;
  const rect = wrap.getBoundingClientRect();
  list.style.top = `${rect.bottom + 4}px`;
  list.style.left = `${rect.left}px`;
  list.style.width = `${rect.width}px`;
}

function _renderAdminEmpResults(query) {
  const list = document.getElementById('admin-emp-results');
  if (!list) return;
  const q = (query || '').trim().toLowerCase();
  if (!q) {
    list.hidden = true;
    return;
  }
  const matches = ORG_DATA.filter(r =>
    (r.nameZh || '').toLowerCase().includes(q) ||
    (r.nameEn || '').toLowerCase().includes(q) ||
    (r.id || '').toLowerCase().includes(q) ||
    (r.dept || '').toLowerCase().includes(q)
  ).slice(0, 8);

  if (matches.length === 0) {
    list.innerHTML = '<div class="admin-emp-search-empty p-small muted">沒有符合的員工</div>';
  } else {
    list.innerHTML = matches.map(m => `
      <button type="button" class="admin-emp-search-item" data-emp-id="${m.id}">
        <span class="avatar">${m.nameZh[0] || '?'}</span>
        <div class="admin-emp-search-item-info">
          <div class="p-medium">${m.nameZh}<span class="muted-inline">  ·  ${m.nameEn || ''}</span></div>
          <div class="p-mini muted">${m.id}  ·  ${m.dept} / ${m.title}</div>
        </div>
      </button>
    `).join('');
  }
  list.hidden = false;
  _positionAdminEmpResults();
}

function openAdminAddModal() {
  const modal = document.getElementById('admin-add-modal');
  if (!modal) return;
  _adminSelectedEmp = null;
  document.getElementById('admin-emp-input').value = '';
  document.getElementById('admin-emp-results').hidden = true;
  // Reset granular permissions to the default (view-all)
  _adminAddPermDraft = _defaultAdminPerms();
  _renderAdminPermListInto(document.getElementById('admin-add-perm-list'), _adminAddPermDraft);
  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  if (window.lucide?.createIcons) lucide.createIcons();
  setTimeout(() => document.getElementById('admin-emp-input')?.focus(), 50);
}

function closeAdminAddModal() {
  const modal = document.getElementById('admin-add-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
}

function initAdminManagement() {
  const tbody = document.getElementById('admin-tbody');
  if (!tbody) return;

  // Render initial 預覽/編輯 counts for all admins
  _renderAllAdminSummaries();
  initAdminPermModal();

  // Edit-permission button — opens the granular perm modal
  tbody.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-admin-action="edit-perm"]');
    if (!btn) return;
    const id = btn.closest('tr')?.dataset.adminId;
    if (id) openAdminPermModal(id);
  });

  // Row checkbox toggle
  tbody.addEventListener('click', (e) => {
    const cb = e.target.closest('.admin-row-check');
    if (!cb) return;
    e.stopPropagation();
    const row = cb.closest('tr');
    const id = row?.dataset.adminId;
    if (!id) return;
    if (adminSelectedIds.has(id)) adminSelectedIds.delete(id);
    else adminSelectedIds.add(id);
    _adminSyncBulkBar();
  });

  // Per-row delete (icon button) — confirm dialog before removal
  tbody.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-admin-action="remove"]');
    if (!btn) return;
    const row = btn.closest('tr');
    const id = row?.dataset.adminId;
    const name = row.querySelector('.workspace-member .p-medium')?.textContent.trim() || '';
    confirmDialog({
      title: `確定刪除管理員「${name}」?`,
      desc: '刪除後該成員將無法進入後台管理介面,但個人帳號與員工資料仍會保留。',
      confirmText: '確定刪除',
      onConfirm: () => {
        row.remove();
        adminSelectedIds.delete(id);
        _adminSyncBulkBar();
        showToast?.({ title: `已刪除管理員「${name}」`, variant: 'success' });
      }
    });
  });

  // Master select-all (toggles all currently-listed rows)
  document.getElementById('admin-select-all')?.addEventListener('click', () => {
    const rows = [...tbody.querySelectorAll('tr')];
    const allChecked = rows.every(r => adminSelectedIds.has(r.dataset.adminId));
    rows.forEach(r => {
      const id = r.dataset.adminId;
      if (!id) return;
      if (allChecked) adminSelectedIds.delete(id);
      else adminSelectedIds.add(id);
    });
    _adminSyncBulkBar();
  });
  document.getElementById('admin-select-all')?.addEventListener('keydown', (e) => {
    if (e.key === ' ' || e.key === 'Enter') {
      e.preventDefault();
      document.getElementById('admin-select-all').click();
    }
  });

  // Bulk cancel — clear selections
  document.getElementById('admin-bulk-cancel')?.addEventListener('click', () => {
    adminSelectedIds.clear();
    _adminSyncBulkBar();
  });

  // Bulk delete — confirm before removing all selected rows
  document.getElementById('admin-bulk-delete')?.addEventListener('click', () => {
    const count = adminSelectedIds.size;
    if (count === 0) return;
    confirmDialog({
      title: `確定刪除 ${count} 位管理員?`,
      desc: '刪除後這些成員將無法進入後台管理介面,但個人帳號與員工資料仍會保留。',
      confirmText: '確定刪除',
      onConfirm: () => {
        adminSelectedIds.forEach(id => {
          tbody.querySelector(`tr[data-admin-id="${id}"]`)?.remove();
        });
        adminSelectedIds.clear();
        _adminSyncBulkBar();
        showToast?.({ title: `已刪除 ${count} 位管理員`, variant: 'success' });
      }
    });
  });

  // 新增管理員 button
  document.getElementById('admin-add-btn')?.addEventListener('click', openAdminAddModal);

  // Modal handlers
  const modal = document.getElementById('admin-add-modal');
  if (!modal) return;
  document.getElementById('admin-add-close')?.addEventListener('click', closeAdminAddModal);
  document.getElementById('admin-add-cancel')?.addEventListener('click', closeAdminAddModal);
  modal.addEventListener('click', (e) => { if (e.target === modal) closeAdminAddModal(); });

  // Search input — filter results
  document.getElementById('admin-emp-input')?.addEventListener('input', (e) => {
    _adminSelectedEmp = null;
    _renderAdminEmpResults(e.target.value);
  });
  // Reposition the floating list on window resize / modal-body scroll
  const repositionIfOpen = () => {
    const list = document.getElementById('admin-emp-results');
    if (list && !list.hidden) _positionAdminEmpResults();
  };
  window.addEventListener('resize', repositionIfOpen);
  modal.querySelector('.modal-body')?.addEventListener('scroll', repositionIfOpen, { passive: true });
  // Click outside results closes them
  document.addEventListener('click', (e) => {
    if (!e.target.closest('.admin-emp-search')) {
      const results = document.getElementById('admin-emp-results');
      if (results) results.hidden = true;
    }
  });

  // Result item click — populate input + remember selection
  document.getElementById('admin-emp-results')?.addEventListener('click', (e) => {
    const item = e.target.closest('.admin-emp-search-item');
    if (!item) return;
    const id = item.dataset.empId;
    const emp = ORG_DATA.find(r => r.id === id);
    if (!emp) return;
    _adminSelectedEmp = emp;
    document.getElementById('admin-emp-input').value = `${emp.nameZh} (${emp.id})`;
    document.getElementById('admin-emp-results').hidden = true;
  });

  // Submit
  document.getElementById('admin-add-submit')?.addEventListener('click', () => {
    if (!_adminSelectedEmp) {
      showToast?.({ title: '請先選擇員工', variant: 'warning' });
      return;
    }
    // Persist the chosen perms against the employee's id so the same admin
    // can be edited later via the pencil button.
    if (_adminAddPermDraft) _ADMIN_PERMS[_adminSelectedEmp.id] = { ..._adminAddPermDraft };
    closeAdminAddModal();
    showToast?.({
      title: `已新增管理員「${_adminSelectedEmp.nameZh}」`,
      variant: 'success'
    });
  });

  // Wire change events on the add-modal's perm list (once)
  _bindAdminPermListEvents(document.getElementById('admin-add-perm-list'), () => _adminAddPermDraft);
}

// ---------- Organization (組織架構) ----------
const ORG_DATA_INITIAL = [
  // L1
  { id: 'IKL001', nameZh: '王偉',     nameEn: 'Wei Wang',       dept: '總經理室',   title: '總經理',                level: 1, isManager: true,  parentId: '__none__' },
  // L2
  { id: 'IKL002', nameZh: '張琪',     nameEn: 'Chi Chang',      dept: '產品開發部', title: 'VP Engineering',        level: 2, isManager: true,  parentId: 'IKL001' },
  { id: 'IKL003', nameZh: '陳俊',     nameEn: 'Jun Chen',       dept: '業務部',     title: 'VP Sales',              level: 2, isManager: true,  parentId: 'IKL001' },
  { id: 'IKL004', nameZh: '李雅琳',   nameEn: 'Yalin Lee',      dept: '行銷部',     title: 'Marketing Director',    level: 2, isManager: true,  parentId: 'IKL001' },
  { id: 'IKL005', nameZh: '黃志明',   nameEn: 'Zhi-Ming Huang', dept: '管理部',     title: 'COO',                   level: 2, isManager: true,  parentId: 'IKL001' },
  // L3
  { id: 'IKL010', nameZh: '林小琪',   nameEn: 'Xiaoqi Lin',     dept: '產品開發部', title: 'Engineering Manager',   level: 3, isManager: true,  parentId: 'IKL002' },
  { id: 'IKL011', nameZh: '王思怡',   nameEn: 'Siyi Wang',      dept: '產品開發部', title: 'Design Manager',        level: 3, isManager: true,  parentId: 'IKL002' },
  { id: 'IKL012', nameZh: '吳瑞利',   nameEn: 'Riley Wu',       dept: '業務部',     title: 'Sales Manager',         level: 3, isManager: true,  parentId: 'IKL003' },
  { id: 'IKL013', nameZh: '陳威霖',   nameEn: 'Wei-Lin Chen',   dept: '業務部',     title: 'Account Manager',       level: 3, isManager: true,  parentId: 'IKL003' },
  { id: 'IKL014', nameZh: '許琳達',   nameEn: 'Linda Hsu',      dept: '行銷部',     title: 'Marketing Manager',     level: 3, isManager: true,  parentId: 'IKL004' },
  { id: 'IKL015', nameZh: '吳勝利',   nameEn: 'Sheng-Li Wu',    dept: '管理部',     title: 'Finance Manager',       level: 3, isManager: true,  parentId: 'IKL005' },
  { id: 'IKL016', nameZh: '張文君',   nameEn: 'Wenjun Chang',   dept: '管理部',     title: 'HR Manager',            level: 3, isManager: true,  parentId: 'IKL005' },
  // L4
  { id: 'IKL020', nameZh: '陳怡如',   nameEn: 'Tammy Chen',     dept: '產品開發部', title: 'Product Manager',       level: 4, isManager: false, parentId: 'IKL010' },
  { id: 'IKL021', nameZh: '鄭威',     nameEn: 'Wei Cheng',      dept: '產品開發部', title: 'Senior Engineer',       level: 4, isManager: false, parentId: 'IKL010' },
  { id: 'IKL022', nameZh: '林俊宏',   nameEn: 'John Lin',       dept: '產品開發部', title: 'Frontend Engineer',     level: 4, isManager: false, parentId: 'IKL010' },
  { id: 'IKL023', nameZh: '王怡蘭',   nameEn: 'Ella Wang',      dept: '產品開發部', title: 'UX Designer',           level: 4, isManager: false, parentId: 'IKL011' },
  { id: 'IKL024', nameZh: '張智涵',   nameEn: 'Zhihan Chang',   dept: '產品開發部', title: 'UI Designer',           level: 4, isManager: false, parentId: 'IKL011' },
  { id: 'IKL025', nameZh: '佐藤健次', nameEn: 'Kenji Sato',     dept: '業務部',     title: 'Account Executive',     level: 4, isManager: false, parentId: 'IKL012' },
  { id: 'IKL026', nameZh: '謝佳玲',   nameEn: 'Jialing Hsieh',  dept: '業務部',     title: 'Account Executive',     level: 4, isManager: false, parentId: 'IKL012' },
  { id: 'IKL027', nameZh: '陳怡君',   nameEn: 'Yi-Chun Chen',   dept: '行銷部',     title: 'Marketing Specialist',  level: 4, isManager: false, parentId: 'IKL014' },
  { id: 'IKL028', nameZh: '林書豪',   nameEn: 'Shuhao Lin',     dept: '行銷部',     title: 'Content Strategist',    level: 4, isManager: false, parentId: 'IKL014' },
  { id: 'IKL029', nameZh: '陳大文',   nameEn: 'Pat Chen',       dept: '管理部',     title: 'Finance Specialist',    level: 4, isManager: false, parentId: 'IKL015' },
  { id: 'IKL030', nameZh: '林雅芬',   nameEn: 'Yafen Lin',      dept: '管理部',     title: 'HR Specialist',         level: 4, isManager: false, parentId: 'IKL016' },
];

let ORG_DATA = ORG_DATA_INITIAL.map(r => ({ ...r }));
let _orgEditing = null;
// Sort state for the table mode: { key: 'id'|'level'|'isManager'|null, dir: 'asc'|'desc' }
let _orgSort = { key: null, dir: 'asc' };

function initOrganization() {
  if (!document.getElementById('org-table-body')) return;
  _orgEditing = ORG_DATA.map(r => ({
    ...r,
    // Default deputy = upper manager. Once set, it's a real value (not auto-tracking parent).
    deputyId: r.deputyId ?? (r.parentId && r.parentId !== ORG_PARENT_NONE ? r.parentId : null),
  }));
  _initOrgPositions();
  renderOrgTable();
  renderOrgTree();  // pre-render so the default-active tree tab has content

  // Mode tabs
  document.querySelectorAll('[data-org-mode]').forEach(tab => {
    tab.addEventListener('click', () => {
      const mode = tab.dataset.orgMode;
      document.querySelectorAll('[data-org-mode]').forEach(t => t.classList.toggle('active', t === tab));
      document.querySelectorAll('[data-org-pane]').forEach(p => p.hidden = p.dataset.orgPane !== mode);
      if (mode === 'tree') renderOrgTree();
      iconsRefresh();
    });
  });

  // 新增部門 — opens the dept rename / new dialog
  $('#org-add-row')?.addEventListener('click', openOrgDeptModal);
  initOrgDeptModal();

  // 發佈 — push the in-memory edits live (mock: confirm + toast)
  $('#org-publish-btn')?.addEventListener('click', () => {
    confirmDialog({
      title: '確定發佈組織異動?',
      desc: '發佈後將更新全公司的部門 / 職缺 / 上下級關係,請假與簽核流程會立即依此運作。',
      confirmText: '確定發佈',
      onConfirm: () => {
        ORG_DATA = _orgEditing.map(r => ({ ...r }));
        showToast?.({ title: '組織異動已發佈', variant: 'success' });
      }
    });
  });

  // Zoom controls (- / 100% / +)
  initOrgChartZoom();

  // 下載組織圖 — capture .org-chart-card as PNG via html2canvas.
  // html2canvas is loaded on-demand (deferred from initial page load) to avoid
  // the ~50KB cost on first paint when most users never download the chart.
  $('#org-tree-download')?.addEventListener('click', async () => {
    const target = document.querySelector('.org-chart-card');
    const chart = document.getElementById('org-tree-render');
    if (!target) return;

    // Lazy-load html2canvas on first click; cached by the browser thereafter.
    if (!window.html2canvas) {
      showToast?.({ title: '正在載入圖片擷取套件...', variant: 'info' });
      try {
        await new Promise((resolve, reject) => {
          const s = document.createElement('script');
          s.src = 'https://cdn.jsdelivr.net/npm/html2canvas@1.4.1/dist/html2canvas.min.js';
          s.onload = resolve;
          s.onerror = () => reject(new Error('script load failed'));
          document.head.appendChild(s);
        });
      } catch {
        showToast?.({ title: '無法載入擷取套件', variant: 'error' });
        return;
      }
    }

    showToast?.({ title: '正在產生圖片...', variant: 'info' });
    const prevTransform = chart?.style.transform || '';
    if (chart) chart.style.transform = '';
    const bg = getComputedStyle(document.documentElement).getPropertyValue('--background').trim() || '#ffffff';
    html2canvas(target, { backgroundColor: bg, scale: 2, useCORS: true }).then(canvas => {
      if (chart) chart.style.transform = prevTransform;
      canvas.toBlob(blob => {
        if (!blob) return;
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `組織架構_${new Date().toISOString().slice(0, 10)}.png`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        setTimeout(() => URL.revokeObjectURL(url), 0);
        showToast?.({ title: '組織圖已下載', variant: 'success' });
      }, 'image/png');
    }).catch(() => {
      if (chart) chart.style.transform = prevTransform;
      showToast?.({ title: '產生圖片失敗', variant: 'error' });
    });
  });

  // Delegated handlers on table body
  const body = $('#org-table-body');
  if (!body) return;

  body.addEventListener('input', (e) => {
    const el = e.target.closest('[data-org-field]');
    if (!el) return;
    const id = el.dataset.orgId;
    const field = el.dataset.orgField;
    const row = _orgEditing.find(r => r.id === id);
    if (!row) return;
    if (field === 'dept') row.dept = el.value;
    else if (field === 'title') row.title = el.value;
    else if (field === 'parentId') row.parentId = el.value || null;
  });
  // <select> elements fire `change`, not `input`, so handle both
  body.addEventListener('change', (e) => {
    const el = e.target.closest('[data-org-field]');
    if (!el) return;
    const id = el.dataset.orgId;
    const field = el.dataset.orgField;
    const row = _orgEditing.find(r => r.id === id);
    if (!row) return;
    if (field === 'dept') {
      row.dept = el.value;
      // Clear title if it's not valid for the new dept (positions are dept-scoped)
      const validPositions = _orgPositions.get(row.dept) || new Set();
      if (row.title && !validPositions.has(row.title)) row.title = '';
      renderOrgTable();  // re-render: dept change shifts candidate parents AND title options
      renderOrgTree();
    } else if (field === 'parentId') {
      renderOrgTree();
    }
  });

  // Row-level edit icon → open the employee edit modal
  body.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-org-edit]');
    if (!btn) return;
    const id = btn.dataset.orgEdit;
    const row = _orgEditing.find(r => r.id === id);
    if (row) _openEmpEditFromOrg(row);
  });

  // Sortable column headers (員編 / 層級 / 主管)
  document.querySelectorAll('.org-edit-table th[data-org-sort]').forEach(th => {
    th.addEventListener('click', () => {
      const key = th.dataset.orgSort;
      if (_orgSort.key === key) {
        _orgSort.dir = _orgSort.dir === 'asc' ? 'desc' : 'asc';
      } else {
        _orgSort = { key, dir: 'asc' };
      }
      _applyOrgSort();
      renderOrgTable();
    });
  });

  // 代理人 popover (single global popover reused across rows)
  initOrgDeputyPopover();
}

// Open the employee edit modal for an org-list row. Matches a real employee
// tbody row by name when possible; otherwise builds a synthetic row from
// the ORG_DATA entry so the modal can pre-fill its fields.
function _openEmpEditFromOrg(orgRow) {
  const employeeRows = document.querySelectorAll('.employee-tbody tr[data-emp-id]');
  for (const r of employeeRows) {
    const name = r.querySelector('.workspace-member .p-medium')?.textContent.trim();
    if (name && (name === orgRow.nameEn || name === orgRow.nameZh)) {
      openEmpModal('edit', r, { section: 'history' });
      return;
    }
  }
  const fake = document.createElement('tr');
  fake.dataset.empId = orgRow.id;
  fake.innerHTML = `
    <td></td>
    <td>${orgRow.id}</td>
    <td>
      <div class="workspace-member">
        <span class="avatar">${(orgRow.nameZh || '?')[0]}</span>
        <div>
          <div class="p-medium">${orgRow.nameZh || ''}</div>
        </div>
      </div>
    </td>
    <td class="emp-position">
      <div>${orgRow.dept || ''}</div>
      <div class="p-mini">${orgRow.title || ''}</div>
    </td>
    <td>正職</td>
    <td></td>
    <td><span class="badge">在職</span></td>
    <td></td>
  `;
  openEmpModal('edit', fake, { section: 'history' });
}

function _applyOrgSort() {
  if (!_orgSort.key) return;
  const dir = _orgSort.dir === 'desc' ? -1 : 1;

  // Resolve the sort value for special keys that map IDs → display text so
  // 上級 / 代理人 sort by the *name* the user actually sees, not the raw id.
  // Rows with no supervisor / deputy sort to the end regardless of direction
  // by appending a high unicode sentinel — keeps "— 未指派 —" out of the way.
  const UNASSIGNED = '￿';
  const valueFor = (row) => {
    const key = _orgSort.key;
    if (key === 'parent') {
      if (row.parentId === ORG_PARENT_NONE) return UNASSIGNED + '0';   // 無上級
      if (!row.parentId) return UNASSIGNED + '1';                       // 未指派
      return _orgEditing.find(c => c.id === row.parentId)?.nameZh ?? UNASSIGNED;
    }
    if (key === 'deputy') {
      let id = row.deputyId
        || (row.parentId && row.parentId !== ORG_PARENT_NONE ? row.parentId : null);
      if (!id) return UNASSIGNED;
      return _orgEditing.find(c => c.id === id)?.nameZh ?? UNASSIGNED;
    }
    if (key === 'dept') {
      // Sort by 部門 (primary). Rows without a dept sort to the end.
      return (row.dept && row.dept.trim()) ? row.dept : UNASSIGNED;
    }
    return row[key];
  };

  _orgEditing.sort((a, b) => {
    let va = valueFor(a);
    let vb = valueFor(b);
    if (typeof va === 'boolean') { va = va ? 1 : 0; vb = vb ? 1 : 0; }
    if (typeof va === 'string' && typeof vb === 'string') {
      return va.localeCompare(vb, 'zh-Hant') * dir;
    }
    if (va < vb) return -1 * dir;
    if (va > vb) return  1 * dir;
    return 0;
  });
}

function _orgMaxLevel() {
  if (!_orgEditing?.length) return 0;
  return Math.max(..._orgEditing.map(r => r.level));
}

// Sentinel value used in parentId to mean "explicitly no upper manager"
// (e.g. 總經理 / CEO at the top of the chart). Distinct from null/empty
// which means "未指派" — i.e. user hasn't picked yet.
const ORG_PARENT_NONE = '__none__';

// Treat a row as "未指派" / incomplete when key fields are blank.
// 無上級(parentId === ORG_PARENT_NONE)is an explicit choice, not 未指派.
function _isOrgRowIncomplete(r) {
  if (!r.dept || !String(r.dept).trim()) return true;
  if (!r.parentId) return true;  // null / undefined / '' all count as 未指派
  return false;
}

// ---------- Org chart zoom (preview pane) ----------
let _orgZoom = 1;
const ORG_ZOOM_MIN = 0.5;
const ORG_ZOOM_MAX = 2;
const ORG_ZOOM_STEP = 0.1;

function _applyOrgZoom() {
  const chart = document.getElementById('org-tree-render');
  const label = document.getElementById('org-zoom-label');
  if (chart) chart.style.transform = `scale(${_orgZoom})`;
  if (label) label.textContent = `${Math.round(_orgZoom * 100)}%`;
}

function initOrgChartZoom() {
  document.getElementById('org-zoom-out')?.addEventListener('click', () => {
    _orgZoom = Math.max(ORG_ZOOM_MIN, +(_orgZoom - ORG_ZOOM_STEP).toFixed(2));
    _applyOrgZoom();
  });
  document.getElementById('org-zoom-in')?.addEventListener('click', () => {
    _orgZoom = Math.min(ORG_ZOOM_MAX, +(_orgZoom + ORG_ZOOM_STEP).toFixed(2));
    _applyOrgZoom();
  });
  document.getElementById('org-zoom-reset')?.addEventListener('click', () => {
    _orgZoom = 1;
    _applyOrgZoom();
  });

  const viewport = document.querySelector('.org-chart-viewport');
  if (!viewport) return;

  // Ctrl/Cmd + scroll to zoom (only when hovering the chart viewport)
  viewport.addEventListener('wheel', (e) => {
    if (!(e.ctrlKey || e.metaKey)) return;
    e.preventDefault();
    const dir = e.deltaY > 0 ? -1 : 1;
    _orgZoom = Math.max(ORG_ZOOM_MIN, Math.min(ORG_ZOOM_MAX, +(_orgZoom + dir * ORG_ZOOM_STEP).toFixed(2)));
    _applyOrgZoom();
  }, { passive: false });

  // Click-and-drag to pan the chart inside the scrollable viewport
  let isDragging = false;
  let startX = 0, startY = 0;
  let startScrollLeft = 0, startScrollTop = 0;

  viewport.addEventListener('mousedown', (e) => {
    // Don't hijack drag from interactive elements (buttons, links, etc.)
    if (e.target.closest('button, a, input, select, [role="switch"], [role="checkbox"]')) return;
    if (e.button !== 0) return;  // primary button only
    isDragging = true;
    startX = e.clientX;
    startY = e.clientY;
    startScrollLeft = viewport.scrollLeft;
    startScrollTop = viewport.scrollTop;
    viewport.classList.add('is-dragging');
  });

  // Listen on the document so a fast drag that leaves the card still tracks
  document.addEventListener('mousemove', (e) => {
    if (!isDragging) return;
    e.preventDefault();
    viewport.scrollLeft = startScrollLeft - (e.clientX - startX);
    viewport.scrollTop  = startScrollTop  - (e.clientY - startY);
  });
  const stopDrag = () => {
    if (!isDragging) return;
    isDragging = false;
    viewport.classList.remove('is-dragging');
  };
  document.addEventListener('mouseup', stopDrag);
  document.addEventListener('mouseleave', stopDrag);
}

function renderOrgTable() {
  const body = $('#org-table-body');
  if (!body) return;

  // Sync sort indicators on headers (the table thead doesn't get re-rendered,
  // so we update the .sort-asc / .sort-desc state in place)
  document.querySelectorAll('.org-edit-table th[data-org-sort]').forEach(th => {
    th.classList.remove('sort-asc', 'sort-desc');
    if (_orgSort.key === th.dataset.orgSort) {
      th.classList.add(_orgSort.dir === 'desc' ? 'sort-desc' : 'sort-asc');
    }
  });

  const maxLevel = _orgMaxLevel();

  body.innerHTML = _orgEditing.map(r => {
    // Parent display — name + dept, or 無上級 / 未指派
    let parentText;
    if (r.parentId === ORG_PARENT_NONE) parentText = '無上級';
    else if (!r.parentId) parentText = '— 未指派 —';
    else {
      const p = _orgEditing.find(c => c.id === r.parentId);
      parentText = p ? `${p.nameZh}` : '— 未指派 —';
    }

    // Deputy display — defaults to parent when unset
    let deputyText;
    if (r.deputyId) {
      const d = _orgEditing.find(c => c.id === r.deputyId);
      deputyText = d ? d.nameZh : '— 未指派 —';
    } else if (r.parentId && r.parentId !== ORG_PARENT_NONE) {
      const p = _orgEditing.find(c => c.id === r.parentId);
      deputyText = p ? p.nameZh : '— 未指派 —';
    } else {
      deputyText = '— 未指派 —';
    }

    const incomplete = _isOrgRowIncomplete(r);
    return `
      <tr data-org-id="${r.id}"${incomplete ? ' class="is-incomplete"' : ''}>
        <td class="org-cell-level">
          <span class="org-dept-level-badge${r.level === maxLevel ? ' org-level-bottom' : ''}" data-level="${r.level}">層級 ${r.level}</span>
        </td>
        <td class="org-cell-empid p-small muted">${r.id}</td>
        <td class="org-cell-name">
          <div>${r.nameZh}</div>
          ${r.nameEn ? `<div class="p-mini muted">${r.nameEn}</div>` : ''}
        </td>
        <td class="org-cell-position">
          <div>${r.dept || '<span class="muted">— 未指派 —</span>'}</div>
          ${r.title ? `<div class="p-mini muted">${r.title}</div>` : ''}
        </td>
        <td class="org-cell-manager">${r.isManager ? '是' : '<span class="muted">否</span>'}</td>
        <td class="org-cell-parent">${parentText === '— 未指派 —' ? `<span class="muted">${parentText}</span>` : parentText}</td>
        <td class="org-cell-deputy">${deputyText === '— 未指派 —' ? `<span class="muted">${deputyText}</span>` : deputyText}</td>
        <td class="org-cell-action text-right">
          <button class="icon-button" type="button" data-org-edit="${r.id}" aria-label="編輯員工資料">
            <i data-lucide="pencil" class="icon"></i>
          </button>
        </td>
      </tr>
    `;
  }).join('');
  iconsRefresh();
}

// ---------- 代理人 (deputy) — searchable per-row picker ----------
//
// Default behavior: when a row's deputyId is unset, show the parent (上級)
// as the default deputy. Once the user explicitly picks someone (including
// re-picking the parent), deputyId is stored and no longer auto-tracks
// changes to parentId.
//
// A single global popover (#org-deputy-popover) is reused across rows;
// it's re-anchored on each open via getBoundingClientRect so it floats above
// any scroll-clipped containers.

let _orgDeputyTriggerId = null;  // id of the row whose deputy is being edited

function _orgGetDeputyDisplay(r) {
  // Returns { text, isDefault } — isDefault is true only when the field is unset (未指派)
  if (r.deputyId) {
    const d = _orgEditing.find(c => c.id === r.deputyId);
    return d ? { text: d.nameZh, isDefault: false } : { text: '— 未指派 —', isDefault: true };
  }
  return { text: '— 未指派 —', isDefault: true };
}

function _orgRenderDeputyTrigger(r) {
  const { text, isDefault } = _orgGetDeputyDisplay(r);
  return `
    <button type="button" class="org-deputy-trigger" data-org-deputy-trigger data-org-id="${r.id}">
      <span class="org-deputy-text${isDefault ? ' is-default' : ''}">${text}</span>
      <i data-lucide="chevron-down" class="icon"></i>
    </button>
  `;
}

function _positionOrgDeputyPopover() {
  const trigger = document.querySelector(`.org-deputy-trigger[data-org-id="${_orgDeputyTriggerId}"]`);
  const pop = document.getElementById('org-deputy-popover');
  if (!trigger || !pop) return;
  const rect = trigger.getBoundingClientRect();
  // Default: open below; if not enough room, open above
  const popHeight = 320;  // approximate max
  const spaceBelow = window.innerHeight - rect.bottom;
  const top = spaceBelow < popHeight && rect.top > popHeight
    ? rect.top - popHeight - 4
    : rect.bottom + 4;
  // Right-align if popover would overflow the viewport on the right
  const popWidth = 280;
  const left = rect.left + popWidth > window.innerWidth - 8
    ? Math.max(8, window.innerWidth - popWidth - 8)
    : rect.left;
  pop.style.top = `${top}px`;
  pop.style.left = `${left}px`;
}

function _orgRenderDeputyResults(query) {
  const list = document.getElementById('org-deputy-results');
  if (!list || !_orgDeputyTriggerId) return;
  const row = _orgEditing.find(r => r.id === _orgDeputyTriggerId);
  if (!row) return;

  const q = (query || '').trim().toLowerCase();
  // Eligible deputies: anyone except the row itself
  let candidates = _orgEditing.filter(c => c.id !== row.id);
  if (q) {
    candidates = candidates.filter(c =>
      (c.nameZh || '').toLowerCase().includes(q) ||
      (c.nameEn || '').toLowerCase().includes(q) ||
      (c.id || '').toLowerCase().includes(q) ||
      (c.dept || '').toLowerCase().includes(q)
    );
  }
  candidates = candidates.slice(0, 50);

  const items = candidates.map(c => `
    <button type="button" class="org-deputy-popover-item${row.deputyId === c.id ? ' is-active' : ''}" data-org-deputy-id="${c.id}">
      <span class="avatar">${(c.nameZh || '?')[0]}</span>
      <div class="org-deputy-popover-item-info">
        <div class="p-medium">${c.nameZh}</div>
        <div class="p-mini muted">${c.id} · ${c.dept || '—'} / ${c.title || '—'}</div>
      </div>
    </button>
  `).join('');

  list.innerHTML = candidates.length
    ? items
    : '<div class="org-deputy-popover-empty">沒有符合的人選</div>';
}

function openOrgDeputyPopover(triggerId) {
  _orgDeputyTriggerId = triggerId;
  const pop = document.getElementById('org-deputy-popover');
  const input = document.getElementById('org-deputy-search-input');
  if (!pop) return;
  // Mark the trigger as open
  document.querySelectorAll('.org-deputy-trigger.is-open').forEach(el => el.classList.remove('is-open'));
  document.querySelector(`.org-deputy-trigger[data-org-id="${triggerId}"]`)?.classList.add('is-open');
  if (input) input.value = '';
  _orgRenderDeputyResults('');
  pop.hidden = false;
  iconsRefresh();
  _positionOrgDeputyPopover();
  setTimeout(() => input?.focus(), 0);
}

function closeOrgDeputyPopover() {
  const pop = document.getElementById('org-deputy-popover');
  if (!pop) return;
  pop.hidden = true;
  document.querySelectorAll('.org-deputy-trigger.is-open').forEach(el => el.classList.remove('is-open'));
  _orgDeputyTriggerId = null;
}

function initOrgDeputyPopover() {
  const pop = document.getElementById('org-deputy-popover');
  if (!pop) return;

  // Trigger clicks (delegated on the table body)
  document.getElementById('org-table-body')?.addEventListener('click', (e) => {
    const trigger = e.target.closest('[data-org-deputy-trigger]');
    if (!trigger) return;
    e.preventDefault();
    const id = trigger.dataset.orgId;
    if (_orgDeputyTriggerId === id && !pop.hidden) {
      closeOrgDeputyPopover();
    } else {
      openOrgDeputyPopover(id);
    }
  });

  // Search input
  document.getElementById('org-deputy-search-input')?.addEventListener('input', (e) => {
    _orgRenderDeputyResults(e.target.value);
  });

  // Result item click
  pop.addEventListener('click', (e) => {
    const item = e.target.closest('[data-org-deputy-id]');
    if (!item) return;
    const triggerId = _orgDeputyTriggerId;
    if (!triggerId) return;
    const newDeputyId = item.dataset.orgDeputyId || null;
    const row = _orgEditing.find(r => r.id === triggerId);
    if (row) {
      row.deputyId = newDeputyId;
      // Re-render only the affected row's trigger
      const trigger = document.querySelector(`.org-deputy-trigger[data-org-id="${triggerId}"]`);
      if (trigger) {
        const { text, isDefault } = _orgGetDeputyDisplay(row);
        const span = trigger.querySelector('.org-deputy-text');
        if (span) {
          span.textContent = text;
          span.classList.toggle('is-default', isDefault);
        }
      }
    }
    closeOrgDeputyPopover();
  });

  // Click outside closes the popover
  document.addEventListener('click', (e) => {
    if (pop.hidden) return;
    if (e.target.closest('.org-deputy-popover')) return;
    if (e.target.closest('[data-org-deputy-trigger]')) return;
    closeOrgDeputyPopover();
  });

  // Escape key
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !pop.hidden) closeOrgDeputyPopover();
  });

  // Reposition on scroll / resize while open
  const reposition = () => { if (!pop.hidden) _positionOrgDeputyPopover(); };
  window.addEventListener('resize', reposition);
  window.addEventListener('scroll', reposition, true);  // capture: catches scrolls in nested containers
}

function renderOrgTree() {
  const host = $('#org-tree-render');
  if (!host) return;
  // Build a tree from _orgEditing using parentId. A node is a root when:
  //   - parentId is unset / 未指派
  //   - parentId is the explicit "無上級" sentinel
  //   - parentId points to an id that no longer exists (orphan)
  const byId = Object.fromEntries(_orgEditing.map(r => [r.id, { ...r, children: [] }]));
  const roots = [];
  _orgEditing.forEach(r => {
    const node = byId[r.id];
    const hasRealParent = r.parentId && r.parentId !== ORG_PARENT_NONE && byId[r.parentId];
    if (hasRealParent) byId[r.parentId].children.push(node);
    else roots.push(node);
  });
  const maxLevel = _orgMaxLevel();
  host.innerHTML = roots.length
    ? roots.map(n => _renderOrgChartNode(n, maxLevel)).join('')
    : '<div class="empty-state p-small muted" style="padding:var(--space-2xl);text-align:center">尚無資料</div>';
  iconsRefresh();
}

// ---------- Org structure modal (編輯組織 — 部門 + 職缺) ----------
//
// Two-pane modal: left lists departments (add/edit/delete + click-to-select);
// right lists 職缺 (positions) for the currently-selected dept. The org table's
// 職位 column becomes a <select> populated from this list.
//
// State:
//   _orgPositions:        Map<deptName, Set<positionTitle>>
//   _orgDeptEditing:      dept currently in inline rename mode (or null)
//   _orgPosEditing:       position currently in inline rename mode (or null)
//   _orgSelectedDept:     dept name shown in the right pane
//   _orgDeptSnapshot:     deep snapshot of _orgEditing + _orgPositions for cancel-revert

let _orgPositions = new Map();
let _orgDeptEditing = null;
let _orgPosEditing = null;
let _orgSelectedDept = null;
let _orgDeptSnapshot = null;

function _initOrgPositions() {
  // Seed positions from current employee titles, grouped by dept.
  _orgPositions = new Map();
  _orgEditing.forEach(r => {
    const dept = (r.dept || '').trim();
    const title = (r.title || '').trim();
    if (!dept) return;
    if (!_orgPositions.has(dept)) _orgPositions.set(dept, new Set());
    if (title) _orgPositions.get(dept).add(title);
  });
}

function _getDeptStats() {
  // Build a Map of dept name → { count, level } from _orgEditing.
  // For 層級 we use the MIN level among rows for that dept (highest in
  // the org), since a single dept conceptually has one level.
  const stats = new Map();
  _orgEditing.forEach(r => {
    const name = (r.dept || '').trim();
    if (!name) return;
    const cur = stats.get(name) || { count: 0, level: r.level };
    cur.count++;
    if (r.level < cur.level) cur.level = r.level;
    stats.set(name, cur);
  });
  // Also ensure depts that exist in _orgPositions but have no members still appear
  _orgPositions.forEach((_, name) => {
    if (!stats.has(name)) stats.set(name, { count: 0, level: 2 });
  });
  return stats;
}

function _orgLevelOptions(currentLevel) {
  const levels = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10];
  return levels.map(lv =>
    `<option value="${lv}"${lv === currentLevel ? ' selected' : ''}>層級 ${lv}</option>`
  ).join('');
}

function _renderOrgDeptList() {
  const host = document.getElementById('org-dept-list');
  if (!host) return;
  const stats = _getDeptStats();
  if (stats.size === 0) {
    host.innerHTML = '<div class="p-small muted org-dept-empty">目前還沒有部門</div>';
    if (window.lucide?.createIcons) lucide.createIcons();
    return;
  }
  // Sort by level asc, then by name
  const sorted = [...stats.entries()].sort((a, b) => a[1].level - b[1].level || a[0].localeCompare(b[0]));
  host.innerHTML = sorted.map(([name, s]) => {
    if (_orgDeptEditing === name) {
      return `
        <div class="org-dept-row org-dept-row-editing" data-org-dept-name="${name}">
          <div class="input-wrap org-dept-level-wrap">
            <select class="input org-dept-edit-level">${_orgLevelOptions(s.level)}</select>
          </div>
          <div class="input-wrap org-dept-name-input"><input class="input org-dept-edit-name" value="${name}" autofocus></div>
          <button class="icon-button" type="button" data-org-dept-action="save" aria-label="儲存"><i data-lucide="check" class="icon"></i></button>
          <button class="icon-button" type="button" data-org-dept-action="cancel" aria-label="取消"><i data-lucide="x" class="icon"></i></button>
        </div>
      `;
    }
    const isBottom = s.level === _orgMaxLevel();
    const isSelected = _orgSelectedDept === name;
    return `
      <div class="org-dept-row${isSelected ? ' is-selected' : ''}" data-org-dept-name="${name}">
        <span class="org-dept-level-badge${isBottom ? ' org-level-bottom' : ''}" data-level="${s.level}">層級 ${s.level}</span>
        <div class="org-dept-row-name">${name}</div>
        <button class="icon-button" type="button" data-org-dept-action="edit" aria-label="編輯部門"><i data-lucide="pencil" class="icon"></i></button>
        <button class="icon-button" type="button" data-org-dept-action="delete" aria-label="刪除部門"><i data-lucide="trash-2" class="icon"></i></button>
      </div>
    `;
  }).join('');
  if (window.lucide?.createIcons) lucide.createIcons();
}

function _renderOrgPosList() {
  const host = document.getElementById('org-pos-list');
  const paneTitle = document.getElementById('org-pos-pane-title');
  const addInput = document.getElementById('org-pos-new-name');
  const addBtn = document.getElementById('org-pos-add');
  if (!host) return;

  if (!_orgSelectedDept) {
    if (paneTitle) paneTitle.textContent = '職缺';
    if (addInput) { addInput.disabled = true; addInput.value = ''; }
    if (addBtn) addBtn.disabled = true;
    host.innerHTML = '<div class="org-pos-empty">請從左側選擇部門以管理職缺</div>';
    return;
  }

  if (addInput) addInput.disabled = false;
  if (addBtn) addBtn.disabled = false;

  const positions = [...(_orgPositions.get(_orgSelectedDept) || new Set())].sort((a, b) => a.localeCompare(b));
  if (paneTitle) paneTitle.textContent = `${_orgSelectedDept} 職缺 (${positions.length})`;
  if (positions.length === 0) {
    host.innerHTML = '<div class="org-pos-empty">尚未設定職缺,新增一個試試</div>';
    return;
  }

  host.innerHTML = positions.map(pos => {
    if (_orgPosEditing === pos) {
      return `
        <div class="org-pos-row org-pos-row-editing" data-org-pos-name="${pos}">
          <i data-lucide="user" class="icon org-pos-row-icon"></i>
          <div class="input-wrap org-pos-name-input"><input class="input org-pos-edit-name" value="${pos}" autofocus></div>
          <button class="icon-button" type="button" data-org-pos-action="save" aria-label="儲存"><i data-lucide="check" class="icon"></i></button>
          <button class="icon-button" type="button" data-org-pos-action="cancel" aria-label="取消"><i data-lucide="x" class="icon"></i></button>
        </div>
      `;
    }
    return `
      <div class="org-pos-row" data-org-pos-name="${pos}">
        <i data-lucide="user" class="icon org-pos-row-icon"></i>
        <div class="org-pos-row-name">${pos}</div>
        <button class="icon-button" type="button" data-org-pos-action="edit" aria-label="編輯職缺"><i data-lucide="pencil" class="icon"></i></button>
        <button class="icon-button" type="button" data-org-pos-action="delete" aria-label="刪除職缺"><i data-lucide="trash-2" class="icon"></i></button>
      </div>
    `;
  }).join('');
  if (window.lucide?.createIcons) lucide.createIcons();
}

function _renderOrgModal() {
  _renderOrgDeptList();
  _renderOrgPosList();
}

function openOrgDeptModal() {
  const modal = document.getElementById('org-dept-modal');
  if (!modal) return;
  _orgDeptEditing = null;
  _orgPosEditing = null;
  // Default-select the first dept (alphabetical by level then name)
  const stats = _getDeptStats();
  if (stats.size > 0) {
    const first = [...stats.entries()].sort((a, b) => a[1].level - b[1].level || a[0].localeCompare(b[0]))[0];
    _orgSelectedDept = first[0];
  } else {
    _orgSelectedDept = null;
  }
  // Snapshot both data structures so 取消 can revert mid-session changes
  _orgDeptSnapshot = {
    rows: _orgEditing.map(r => ({ ...r })),
    positions: new Map([..._orgPositions.entries()].map(([k, v]) => [k, new Set(v)])),
  };
  _renderOrgModal();
  document.getElementById('org-dept-new-name').value = '';
  document.getElementById('org-dept-new-level').value = '1';
  document.getElementById('org-pos-new-name').value = '';
  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  if (window.lucide?.createIcons) lucide.createIcons();
}

function closeOrgDeptModal() {
  const modal = document.getElementById('org-dept-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
}

function _orgDeptCancelChanges() {
  if (_orgDeptSnapshot) {
    _orgEditing = _orgDeptSnapshot.rows;
    _orgPositions = _orgDeptSnapshot.positions;
    _orgDeptSnapshot = null;
    renderOrgTable();
    renderOrgTree();
  }
  closeOrgDeptModal();
}

function _orgDeptSaveChanges() {
  _orgDeptSnapshot = null;
  closeOrgDeptModal();
  showToast?.({ title: '部門和職位已儲存', variant: 'success' });
}

function initOrgDeptModal() {
  const modal = document.getElementById('org-dept-modal');
  if (!modal) return;
  document.getElementById('org-dept-modal-close')?.addEventListener('click', _orgDeptCancelChanges);
  document.getElementById('org-dept-modal-cancel')?.addEventListener('click', _orgDeptCancelChanges);
  document.getElementById('org-dept-modal-save')?.addEventListener('click', _orgDeptSaveChanges);
  modal.addEventListener('click', (e) => { if (e.target === modal) _orgDeptCancelChanges(); });

  // --- 新增部門 ---
  document.getElementById('org-dept-add')?.addEventListener('click', () => {
    const nameInput = document.getElementById('org-dept-new-name');
    const levelInput = document.getElementById('org-dept-new-level');
    const name = nameInput.value.trim();
    const level = Math.max(1, parseInt(levelInput.value, 10) || 1);
    if (!name) { nameInput.focus(); return; }
    if (_getDeptStats().has(name)) {
      showToast?.({ title: `部門「${name}」已存在`, variant: 'warning' });
      return;
    }
    const seq = _orgEditing.length + 1;
    _orgEditing.push({
      id: `IKL${String(seq).padStart(3, '0')}`,
      nameZh: '—', nameEn: '', dept: name, title: '',
      level, isManager: false, parentId: null,
    });
    if (!_orgPositions.has(name)) _orgPositions.set(name, new Set());
    nameInput.value = '';
    levelInput.value = '1';
    _orgSelectedDept = name;  // jump to the new dept so user can immediately add positions
    _renderOrgModal();
    renderOrgTable();
    renderOrgTree();
    showToast?.({ title: `已新增部門「${name}」`, variant: 'success' });
  });
  document.getElementById('org-dept-new-name')?.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); document.getElementById('org-dept-add')?.click(); }
  });

  // --- Dept row actions (delegated): select / edit / save / cancel / delete ---
  document.getElementById('org-dept-list')?.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-org-dept-action]');
    const row = e.target.closest('.org-dept-row');
    const name = row?.dataset.orgDeptName;
    if (!name) return;

    if (!btn) {
      // Plain row click → select this dept (right pane shows its positions)
      if (_orgDeptEditing) return;  // don't switch while another row is mid-edit
      _orgSelectedDept = name;
      _renderOrgModal();
      return;
    }

    const action = btn.dataset.orgDeptAction;
    if (action === 'edit') {
      _orgDeptEditing = name;
      _renderOrgDeptList();
    } else if (action === 'cancel') {
      _orgDeptEditing = null;
      _renderOrgDeptList();
    } else if (action === 'save') {
      const newName = row.querySelector('.org-dept-edit-name').value.trim();
      const newLevel = parseInt(row.querySelector('.org-dept-edit-level').value, 10) || 1;
      if (!newName) return;
      if (newName !== name && _getDeptStats().has(newName)) {
        showToast?.({ title: `部門「${newName}」已存在`, variant: 'warning' });
        return;
      }
      _orgEditing.forEach(r => { if (r.dept === name) { r.dept = newName; r.level = newLevel; } });
      // Transfer positions to the new dept name
      if (newName !== name && _orgPositions.has(name)) {
        _orgPositions.set(newName, _orgPositions.get(name));
        _orgPositions.delete(name);
      }
      if (_orgSelectedDept === name) _orgSelectedDept = newName;
      _orgDeptEditing = null;
      _renderOrgModal();
      renderOrgTable();
      renderOrgTree();
      showToast?.({ title: `部門已更新為「${newName}」`, variant: 'success' });
    } else if (action === 'delete') {
      const stats = _getDeptStats();
      const count = stats.get(name)?.count || 0;
      confirmDialog({
        title: `確定刪除部門「${name}」?`,
        desc: count > 0
          ? `此部門目前有 ${count} 位成員,刪除後該部門資料將從成員資料中清空(成員仍會保留)。`
          : '此部門目前沒有成員,可安全刪除。',
        confirmText: '確定刪除',
        onConfirm: () => {
          _orgEditing.forEach(r => { if (r.dept === name) { r.dept = ''; r.title = ''; } });
          _orgPositions.delete(name);
          if (_orgSelectedDept === name) {
            // Move selection to whatever remains, if any
            const stats2 = _getDeptStats();
            _orgSelectedDept = stats2.size > 0
              ? [...stats2.entries()].sort((a, b) => a[1].level - b[1].level || a[0].localeCompare(b[0]))[0][0]
              : null;
          }
          _renderOrgModal();
          renderOrgTable();
          renderOrgTree();
          showToast?.({ title: `已刪除部門「${name}」`, variant: 'success' });
        }
      });
    }
  });

  // --- 新增職缺 ---
  document.getElementById('org-pos-add')?.addEventListener('click', () => {
    if (!_orgSelectedDept) return;
    const input = document.getElementById('org-pos-new-name');
    const name = input.value.trim();
    if (!name) { input.focus(); return; }
    const set = _orgPositions.get(_orgSelectedDept) || new Set();
    if (set.has(name)) {
      showToast?.({ title: `職缺「${name}」已存在`, variant: 'warning' });
      return;
    }
    set.add(name);
    _orgPositions.set(_orgSelectedDept, set);
    input.value = '';
    _renderOrgPosList();
    renderOrgTable();  // title dropdown options changed
    showToast?.({ title: `已新增職缺「${name}」`, variant: 'success' });
  });
  document.getElementById('org-pos-new-name')?.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); document.getElementById('org-pos-add')?.click(); }
  });

  // --- Position row actions ---
  document.getElementById('org-pos-list')?.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-org-pos-action]');
    if (!btn) return;
    const row = btn.closest('.org-pos-row');
    const pos = row?.dataset.orgPosName;
    if (!pos || !_orgSelectedDept) return;
    const action = btn.dataset.orgPosAction;
    const set = _orgPositions.get(_orgSelectedDept);
    if (!set) return;

    if (action === 'edit') {
      _orgPosEditing = pos;
      _renderOrgPosList();
    } else if (action === 'cancel') {
      _orgPosEditing = null;
      _renderOrgPosList();
    } else if (action === 'save') {
      const newName = row.querySelector('.org-pos-edit-name').value.trim();
      if (!newName) return;
      if (newName !== pos && set.has(newName)) {
        showToast?.({ title: `職缺「${newName}」已存在`, variant: 'warning' });
        return;
      }
      set.delete(pos);
      set.add(newName);
      // Update any members currently using the old title within this dept
      _orgEditing.forEach(r => {
        if (r.dept === _orgSelectedDept && r.title === pos) r.title = newName;
      });
      _orgPosEditing = null;
      _renderOrgPosList();
      renderOrgTable();
      showToast?.({ title: `職缺已更新為「${newName}」`, variant: 'success' });
    } else if (action === 'delete') {
      const count = _orgEditing.filter(r => r.dept === _orgSelectedDept && r.title === pos).length;
      confirmDialog({
        title: `確定刪除職缺「${pos}」?`,
        desc: count > 0
          ? `目前有 ${count} 位成員職位為此職缺,刪除後這些成員的職位將清空(成員仍會保留)。`
          : '此職缺目前沒有成員,可安全刪除。',
        confirmText: '確定刪除',
        onConfirm: () => {
          set.delete(pos);
          _orgEditing.forEach(r => { if (r.dept === _orgSelectedDept && r.title === pos) r.title = ''; });
          _renderOrgPosList();
          renderOrgTable();
          showToast?.({ title: `已刪除職缺「${pos}」`, variant: 'success' });
        }
      });
    }
  });
}

// Renders a top-down org chart node — card on top, children row below.
// `maxLevel` is the deepest level present in current data; nodes at that
// level get .is-bottom-level (white card bg) + .org-level-bottom on the tag.
function _renderOrgChartNode(node, maxLevel) {
  const subtitle = node.title || '—';
  const isBottom = node.level === maxLevel;
  const childrenHtml = node.children.length ? `
    <div class="org-chart-children${node.children.length > 1 ? ' has-multiple' : ''}">
      ${node.children.map(c => _renderOrgChartNode(c, maxLevel)).join('')}
    </div>
  ` : '';
  return `
    <div class="org-chart-node">
      <div class="org-chart-card-box${isBottom ? ' is-bottom-level' : ''}" data-level="${node.level}">
        <div class="org-chart-card-name">${node.nameZh}</div>
        ${node.nameEn ? `<div class="org-chart-card-en">${node.nameEn}</div>` : ''}
        <div class="org-chart-card-dept">${node.dept || '—'}</div>
        <div class="org-chart-card-meta">${subtitle}<span class="org-level-tag${isBottom ? ' org-level-bottom' : ''}" data-level="${node.level}">L${node.level}</span></div>
      </div>
      ${childrenHtml}
    </div>
  `;
}

// ---------- Workspace overview month nav + daily leave chart ----------
const _overviewToday = new Date();
let overviewYear = _overviewToday.getFullYear();
let overviewMonth = _overviewToday.getMonth() + 1;  // 1-based

function renderOverviewLeaveChart() {
  const host = document.getElementById('overview-leave-bars');
  if (!host) return;
  const days = new Date(overviewYear, overviewMonth, 0).getDate();  // last day of month
  // Deterministic mock data — same year/month always produces same numbers.
  const data = [];
  for (let d = 1; d <= days; d++) {
    const seed = (overviewYear * 31 + overviewMonth * 7 + d * 13) % 11;
    // Weekend dampening: Sat/Sun fewer leave requests
    const dow = new Date(overviewYear, overviewMonth - 1, d).getDay();
    const weekendFactor = (dow === 0 || dow === 6) ? 0.3 : 1;
    data.push(Math.round(seed * weekendFactor));
  }
  const max = Math.max(...data, 1);
  // Highlight today's bar only if the viewed month is the current month
  const now = new Date();
  const todayDay = (now.getFullYear() === overviewYear && now.getMonth() + 1 === overviewMonth)
    ? now.getDate() : null;
  const dowNames = ['日', '一', '二', '三', '四', '五', '六'];
  host.innerHTML = data.map((v, i) => {
    const day = i + 1;
    const dow = new Date(overviewYear, overviewMonth - 1, day).getDay();
    // Show day label only every 5 days plus the last day for legibility
    const showLabel = day === 1 || day % 5 === 0 || day === days;
    const heightPct = (v / max) * 100;
    const activeClass = day === todayDay ? ' active' : '';
    const tip = `${overviewMonth}/${day}(週${dowNames[dow]})  ·  請假 ${v} 人`;
    return `<div class="dash-bar${activeClass}"${showLabel ? ' data-show' : ''} style="--h:${heightPct}%" data-tip="${tip}"><span>${day}</span></div>`;
  }).join('');
}

function updateOverviewMonth() {
  const label = document.getElementById('overview-month-label');
  if (label) label.textContent = `${overviewYear} 年 ${overviewMonth} 月`;
  renderOverviewLeaveChart();
}

function initOverviewMonthNav() {
  if (!document.getElementById('overview-month-prev')) return;
  document.getElementById('overview-month-prev').addEventListener('click', () => {
    overviewMonth--;
    if (overviewMonth < 1) { overviewMonth = 12; overviewYear--; }
    updateOverviewMonth();
  });
  document.getElementById('overview-month-next').addEventListener('click', () => {
    overviewMonth++;
    if (overviewMonth > 12) { overviewMonth = 1; overviewYear++; }
    updateOverviewMonth();
  });
  updateOverviewMonth();
}

// ---------- Employee management (員工管理) ----------
const EMP_TYPE_TO_VALUE = { '全職': 'full-time', '兼職': 'part-time', '實習': 'intern', '約聘': 'contract', '其他': 'other' };
const EMP_STATUS_VALUES = ['在職', '試用中', '留停', '待加入', '離職'];
const EMP_PAGE_SIZE = 5;
let empPage = 1;
const empSelectedIds = new Set();
const EMP_FILTERS = { query: '', dept: 'all', status: 'all', type: 'all' };

function _empSyncBulkBar() {
  const tbody = document.querySelector('.employee-tbody');
  if (!tbody) return;
  const rows = [...tbody.querySelectorAll('tr')];
  const visible = rows.filter(r => r.style.display !== 'none');
  // Drop selections whose rows have been removed
  rows.forEach(r => {
    const id = r.dataset.empId;
    const cb = r.querySelector('.emp-row-check');
    if (cb) cb.setAttribute('aria-checked', empSelectedIds.has(id) ? 'true' : 'false');
  });
  // Master checkbox tri-state based on currently visible rows
  const all = document.getElementById('emp-select-all');
  if (all) {
    const visIds = visible.map(r => r.dataset.empId);
    const checkedVis = visIds.filter(id => empSelectedIds.has(id)).length;
    if (checkedVis === 0) all.setAttribute('aria-checked', 'false');
    else if (checkedVis === visIds.length) all.setAttribute('aria-checked', 'true');
    else all.setAttribute('aria-checked', 'mixed');
  }
  // Floating bulk action bar
  const bar = document.getElementById('emp-bulk-actionbar');
  const count = document.getElementById('emp-bulk-count');
  if (count) count.textContent = empSelectedIds.size;
  if (bar) bar.hidden = empSelectedIds.size === 0;
}

// Compute the next sequential 員工編號 in the form IKL### (3-digit suffix).
// Looks at all existing rows, finds the highest numeric suffix, returns next.
function _empNextEmployeeId() {
  const rows = document.querySelectorAll('.employee-tbody tr[data-emp-id]');
  let maxSeq = 0;
  rows.forEach(r => {
    const m = (r.dataset.empId || '').match(/^IKL(\d+)$/);
    if (m) {
      const seq = +m[1];
      if (seq > maxSeq) maxSeq = seq;
    }
  });
  return `IKL${String(maxSeq + 1).padStart(3, '0')}`;
}

// Compute tenure ("年資") from a YYYY/MM/DD hire date string vs reference date.
// Returns "N 年" / "N 個月" / "新入職" / "尚未到職".
function computeTenure(hireDateStr, today = new Date()) {
  const m = hireDateStr.match(/^(\d{4})\/(\d{2})\/(\d{2})/);
  if (!m) return '';
  const hire = new Date(+m[1], +m[2] - 1, +m[3]);
  if (hire > today) return '尚未到職';
  // Whole calendar years
  let years = today.getFullYear() - hire.getFullYear();
  const monthDelta = today.getMonth() - hire.getMonth();
  const dayDelta = today.getDate() - hire.getDate();
  if (monthDelta < 0 || (monthDelta === 0 && dayDelta < 0)) years--;
  if (years >= 1) return `${years} 年`;
  // Less than 1 year — fall back to months
  let months = (today.getFullYear() - hire.getFullYear()) * 12 + (today.getMonth() - hire.getMonth());
  if (today.getDate() < hire.getDate()) months--;
  if (months >= 1) return `${months} 個月`;
  return '新入職';
}

function _empParseDate(dateStr) {
  const m = String(dateStr || '').trim().match(/^(\d{4})[\/-](\d{1,2})[\/-](\d{1,2})/);
  if (!m) return null;
  return new Date(+m[1], +m[2] - 1, +m[3]);
}

function _empReadRow(row) {
  const cells = row.cells;
  const statusText = row.querySelector('.badge')?.textContent.trim() || '';
  const typeText = cells[4]?.textContent.trim() || '';
  const hireText = cells[7]?.textContent.trim() || '';
  const leftText = row.dataset.empLeftDate || '';
  const deptCell = row.querySelector('.emp-position');

  return {
    row,
    id: row.dataset.empId || cells[1]?.textContent.trim() || '',
    name: row.querySelector('.workspace-member .p-medium')?.textContent.trim() || '',
    email: row.querySelector('.workspace-member .p-mini')?.textContent.trim() || '',
    dept: deptCell?.children[0]?.textContent.trim() || '',
    title: deptCell?.querySelector('.p-mini')?.textContent.trim() || '',
    typeText,
    type: EMP_TYPE_TO_VALUE[typeText] || 'other',
    status: EMP_STATUS_VALUES.includes(statusText) ? statusText : '',
    hireDateText: hireText.match(/^(\d{4}\/\d{2}\/\d{2})/)?.[1] || '',
    hireDate: _empParseDate(hireText),
    leftDate: _empParseDate(leftText),
  };
}

function _empMatchesFilters(emp) {
  const query = EMP_FILTERS.query.trim().toLowerCase();
  const matchesQuery = !query || [emp.name, emp.email, emp.id].some(v => v.toLowerCase().includes(query));
  const matchesDept = EMP_FILTERS.dept === 'all' || emp.dept === EMP_FILTERS.dept;
  const matchesStatus = EMP_FILTERS.status === 'all' || emp.status === EMP_FILTERS.status;
  const matchesType = EMP_FILTERS.type === 'all' || emp.type === EMP_FILTERS.type;
  return matchesQuery && matchesDept && matchesStatus && matchesType;
}

function _empWireFilterMenu({ empSection, labelSelector, itemAttr, filterKey, defaultLabel, formatLabel }) {
  const dropdown = empSection?.querySelector(labelSelector)?.closest('.dropdown');
  if (!dropdown) return;

  const trigger = dropdown.querySelector('button');
  const labelEl = dropdown.querySelector(labelSelector);
  const itemSelector = `[${itemAttr}]`;

  trigger?.addEventListener('click', (e) => {
    e.stopPropagation();
    toggleDropdownExclusive(dropdown);
  });

  dropdown.querySelectorAll(itemSelector).forEach(item => {
    item.addEventListener('click', () => {
      const value = item.getAttribute(itemAttr) || 'all';
      EMP_FILTERS[filterKey] = value;
      empPage = 1;

      dropdown.querySelectorAll(itemSelector).forEach(i => i.classList.remove('selected'));
      item.classList.add('selected');

      if (labelEl) {
        if (formatLabel) labelEl.textContent = formatLabel(item, value);
        else labelEl.textContent = value === 'all'
          ? defaultLabel
          : (item.querySelector('span')?.textContent.trim() || defaultLabel);
      }
      trigger?.classList.toggle('is-filtered', value !== 'all');
      dropdown.classList.remove('open');
      refreshEmployeeView();
    });
  });

  dropdown.querySelector(`${itemSelector}[${itemAttr}="all"]`)?.classList.add('selected');
}

function refreshEmployeeView() {
  const tbody = document.querySelector('.employee-tbody');
  if (!tbody) return;
  const rows = [...tbody.querySelectorAll('tr')];
  const employees = rows.map(_empReadRow);
  const filteredEmployees = employees.filter(_empMatchesFilters);
  const total = filteredEmployees.length;

  // Aggregate metrics for stat cards + filter dropdown
  const makeStatusStats = () => ({
    counts: { '在職': 0, '試用中': 0, '留停': 0, '待加入': 0, '離職': 0 },
    leftThisMonth: 0,
  });
  const allStats = makeStatusStats();
  const filteredStats = makeStatusStats();
  const typeCounts = { 'full-time': 0, 'part-time': 0, 'intern': 0, 'contract': 0, 'other': 0 };

  const today = new Date();
  const curYear = today.getFullYear();
  const curMonth = today.getMonth();  // 0-based
  const addStatusStat = (emp, stats) => {
    if (emp.status && stats.counts[emp.status] !== undefined) stats.counts[emp.status]++;
    if (
      emp.status === '離職' &&
      emp.leftDate &&
      emp.leftDate.getFullYear() === curYear &&
      emp.leftDate.getMonth() === curMonth
    ) {
      stats.leftThisMonth++;
    }
  };

  employees.forEach(emp => {
    if (typeCounts[emp.type] !== undefined) typeCounts[emp.type]++;
    addStatusStat(emp, allStats);
  });

  filteredEmployees.forEach(emp => {
    addStatusStat(emp, filteredStats);
  });

  // Append tenure (年資) suffix to each hire-date cell
  rows.forEach(r => {
    const cell = r.cells[7];
    if (!cell) return;
    const m = cell.textContent.match(/^(\d{4}\/\d{2}\/\d{2})/);
    if (!m) return;
    const date = m[1];
    const tenure = computeTenure(date, today);
    cell.innerHTML = tenure
      ? `${date} <span class="muted">(${tenure})</span>`
      : date;
  });

  const setStat = (scopeSelector, key, n) => {
    document.querySelectorAll(`${scopeSelector} [data-emp-stat="${key}"]`).forEach(el => {
      el.innerHTML = `${n}<span class="p-small">人</span>`;
    });
  };
  const applyStats = (scopeSelector, totalCount, stats) => {
    setStat(scopeSelector, 'total', totalCount);
    setStat(scopeSelector, 'active', stats.counts['在職']);
    setStat(scopeSelector, 'left-month', stats.leftThisMonth);
    setStat(scopeSelector, 'probation', stats.counts['試用中']);
  };
  applyStats('.workspace-section[data-section="overview"]', employees.length, allStats);
  applyStats('.workspace-section[data-section="employees"]', total, filteredStats);

  // Update 類別 filter dropdown counts
  const setTypeCount = (key, n) => {
    const el = document.querySelector(`[data-emp-type-count="${key}"]`);
    if (el) el.textContent = n;
  };
  setTypeCount('all', employees.length);
  Object.entries(typeCounts).forEach(([k, v]) => setTypeCount(k, v));

  // Clamp page in case rows were removed
  const totalPages = Math.max(1, Math.ceil(total / EMP_PAGE_SIZE));
  if (empPage > totalPages) empPage = totalPages;

  // Show only the rows for the current page
  const start = (empPage - 1) * EMP_PAGE_SIZE;
  rows.forEach(r => { r.style.display = 'none'; });
  filteredEmployees.slice(start, start + EMP_PAGE_SIZE).forEach(emp => {
    emp.row.style.display = '';
  });

  renderPagination({
    hostId: 'emp-pagination',
    totalItems: total,
    page: empPage,
    pageSize: EMP_PAGE_SIZE,
    onPageChange: (p) => { empPage = p; refreshEmployeeView(); }
  });

  // Drop selections that no longer exist in the table; sync UI state
  const liveIds = new Set(rows.map(r => r.dataset.empId));
  [...empSelectedIds].forEach(id => { if (!liveIds.has(id)) empSelectedIds.delete(id); });
  _empSyncBulkBar();
}

function initEmployeeManagement() {
  const tbody = document.querySelector('.employee-tbody');
  if (!tbody) return;

  // Row checkbox toggle
  tbody.addEventListener('click', (e) => {
    const cb = e.target.closest('.emp-row-check');
    if (cb) {
      e.stopPropagation();
      const row = cb.closest('tr');
      const id = row?.dataset.empId;
      if (!id) return;
      if (empSelectedIds.has(id)) empSelectedIds.delete(id);
      else empSelectedIds.add(id);
      _empSyncBulkBar();
    }
  });

  // Row action menu: ellipsis click toggles dropdown; edit / delete dispatch
  tbody.addEventListener('click', (e) => {
    const trigger = e.target.closest('.emp-row-menu > .icon-button');
    if (trigger) {
      e.stopPropagation();
      toggleDropdownExclusive(trigger.closest('.dropdown'));
      return;
    }
    const action = e.target.closest('[data-emp-action]');
    if (!action) return;
    e.stopPropagation();
    const row = action.closest('tr');
    action.closest('.dropdown')?.classList.remove('open');
    const op = action.dataset.empAction;
    if (op === 'view') {
      openEmpModal('view', row);
    } else if (op === 'edit') {
      openEmpModal('edit', row);
    } else if (op === 'delete') {
      const name = row.querySelector('.workspace-member .p-medium')?.textContent.trim() || '';
      confirmDialog({
        title: `確定刪除 ${name}?`,
        desc: '刪除後該成員的個人資料、歷史申請與審核紀錄將一併移除,此動作無法復原。',
        confirmText: '確定刪除',
        onConfirm: () => {
          row.remove();
          refreshEmployeeView();
          showToast?.({ title: `${name} 已刪除`, variant: 'success' });
        }
      });
    }
  });

  // Search + filters — update the table, pagination and stats immediately.
  const empSection = document.querySelector('.workspace-section[data-section="employees"]');
  _empWireFilterMenu({
    empSection,
    labelSelector: '[data-emp-dept-label]',
    itemAttr: 'data-emp-dept',
    filterKey: 'dept',
    defaultLabel: '部門',
  });
  _empWireFilterMenu({
    empSection,
    labelSelector: '[data-emp-status-label]',
    itemAttr: 'data-emp-status',
    filterKey: 'status',
    defaultLabel: '狀態',
  });
  _empWireFilterMenu({
    empSection,
    labelSelector: '[data-emp-type-label]',
    itemAttr: 'data-emp-type',
    filterKey: 'type',
    defaultLabel: '類別',
    formatLabel: (item, value) => {
      if (value === 'all') return '類別';
      const label = item.querySelector('span')?.textContent.trim() || '';
      const count = item.querySelector('[data-emp-type-count]')?.textContent.trim() || '0';
      return `${label}(${count})`;
    },
  });
  empSection?.querySelector('[data-emp-search]')?.addEventListener('input', (e) => {
    EMP_FILTERS.query = e.target.value || '';
    empPage = 1;
    refreshEmployeeView();
  });

  // 新增成員
  $('#emp-add-btn')?.addEventListener('click', () => openEmpModal('add'));

  // 匯出 / 匯入
  $('#emp-export-btn')?.addEventListener('click', exportEmployeeCsv);
  $('#emp-import-btn')?.addEventListener('click', openEmpImportModal);
  initEmpImportModal();

  // Master select-all (toggles selection for currently-visible page only)
  $('#emp-select-all')?.addEventListener('click', () => {
    const visible = [...tbody.querySelectorAll('tr')].filter(r => r.style.display !== 'none');
    const allChecked = visible.every(r => empSelectedIds.has(r.dataset.empId));
    visible.forEach(r => {
      const id = r.dataset.empId;
      if (!id) return;
      if (allChecked) empSelectedIds.delete(id);
      else empSelectedIds.add(id);
    });
    _empSyncBulkBar();
  });
  // Keyboard support on master checkbox
  $('#emp-select-all')?.addEventListener('keydown', (e) => {
    if (e.key === ' ' || e.key === 'Enter') {
      e.preventDefault();
      $('#emp-select-all').click();
    }
  });

  // Bulk cancel — clear all selections
  $('#emp-bulk-cancel')?.addEventListener('click', () => {
    empSelectedIds.clear();
    _empSyncBulkBar();
  });

  // Bulk delete — confirm dialog then remove
  $('#emp-bulk-delete')?.addEventListener('click', () => {
    const count = empSelectedIds.size;
    if (count === 0) return;
    confirmDialog({
      title: `確定刪除 ${count} 位成員?`,
      desc: '刪除後個人資料、歷史申請與審核紀錄將一併移除,此動作無法復原。',
      confirmText: '確定刪除',
      onConfirm: () => {
        empSelectedIds.forEach(id => {
          tbody.querySelector(`tr[data-emp-id="${id}"]`)?.remove();
        });
        empSelectedIds.clear();
        refreshEmployeeView();
        showToast?.({ title: `已刪除 ${count} 位成員`, variant: 'success' });
      }
    });
  });

  // 狀態 select — toggle 離職時間 + 留職停薪 區塊
  $('#emp-f-status')?.addEventListener('change', (e) => {
    const val = e.target.value;
    const resignRow = $('#emp-f-resign-row');
    if (resignRow) resignRow.hidden = val !== 'resigned';
    const leaveRow = $('#emp-f-leave-row');
    if (leaveRow) leaveRow.hidden = val !== 'on-leave';
  });
  // 畢業狀態 select — toggle 畢業日期 / 肄業日期 欄位
  $('#emp-f-edu-status')?.addEventListener('change', (e) => {
    _empSyncEduStatus(e.target.value);
  });

  // 大頭照上傳 — 預設顯示英文首字紫底；hover 浮現更換/移除按鈕
  const photoInput = $('#emp-f-photo');
  const photoPreview = $('#emp-f-photo-preview');
  const photoInitialEl = $('#emp-f-photo-initial');
  const photoEnFirst = $('#emp-f-en-first');
  const photoChName = $('#emp-f-name');

  function _empUpdatePhotoInitial() {
    const en = photoEnFirst?.value.trim() || '';
    const ch = photoChName?.value.trim() || '';
    const letter = (en[0] || ch[0] || '').toUpperCase();
    if (photoInitialEl) photoInitialEl.textContent = letter;
    // data-empty toggles the default person icon ↔ initial letter (see CSS)
    if (photoPreview) {
      if (letter) photoPreview.removeAttribute('data-empty');
      else photoPreview.setAttribute('data-empty', '');
    }
  }
  photoEnFirst?.addEventListener('input', _empUpdatePhotoInitial);
  photoChName?.addEventListener('input', _empUpdatePhotoInitial);
  _empUpdatePhotoInitial();

  function _empSetPhoto(url) {
    if (!photoPreview) return;
    let img = photoPreview.querySelector('img');
    if (url) {
      if (!img) {
        img = document.createElement('img');
        img.alt = '';
        photoPreview.insertBefore(img, photoPreview.firstChild);
      }
      img.src = url;
      photoPreview.dataset.state = 'has-image';
    } else {
      img?.remove();
      photoPreview.dataset.state = 'default';
      if (photoInput) photoInput.value = '';
    }
  }

  photoPreview?.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-emp-photo-action]');
    if (!btn) return;
    if (btn.dataset.empPhotoAction === 'replace') photoInput?.click();
    else if (btn.dataset.empPhotoAction === 'remove') _empSetPhoto(null);
  });

  photoInput?.addEventListener('change', (e) => {
    const file = e.target.files?.[0];
    if (file) _empSetPhoto(URL.createObjectURL(file));
  });

  // Repopulate 職位 options whenever 部門 changes
  $('#emp-f-dept')?.addEventListener('change', () => _empSyncTitleOptions(''));

  // Dept/title hint CTA — close this modal and jump to 組織架構 so the
  // admin can build the department / position catalog before assigning
  // members.
  $('#emp-dept-hint-link')?.addEventListener('click', () => {
    closeEmpModal();
    document.querySelector('[data-workspace-section="organization"]')?.click();
  });

  // Modal handlers
  const modal = $('#emp-modal');
  if (!modal) return;
  $('#emp-modal-close')?.addEventListener('click', closeEmpModal);
  $('#emp-modal-cancel')?.addEventListener('click', closeEmpModal);
  $('#emp-modal-save')?.addEventListener('click', () => {
    const isEdit = $('#emp-modal-title').textContent.trim() === '編輯成員';
    closeEmpModal();
    showToast?.({ title: isEdit ? '已儲存變更' : '已新增成員', variant: 'success' });
  });
  // Switch from preview to edit while keeping the same employee context
  $('#emp-modal-edit-from-view')?.addEventListener('click', () => {
    const row = modal._currentRow;
    if (row) openEmpModal('edit', row);
  });
  modal.addEventListener('click', (e) => {
    if (e.target === modal) closeEmpModal();
  });

  // Sidebar nav — click a section item to switch panes
  modal.querySelectorAll('[data-emp-section]').forEach(btn => {
    btn.addEventListener('click', () => {
      const key = btn.dataset.empSection;
      modal.querySelectorAll('[data-emp-section]').forEach(b => b.classList.toggle('active', b === btn));
      modal.querySelectorAll('[data-emp-pane]').forEach(p => {
        p.hidden = p.dataset.empPane !== key;
      });
      // Scroll the section pane back to top when switching
      modal.querySelector('.emp-modal-sections')?.scrollTo?.({ top: 0 });
    });
  });

  // 本國 / 外國籍 radio — toggle 身分證號 vs 外籍員工資料 block
  modal.querySelectorAll('input[name="emp-f-nat-type"]').forEach(r => {
    r.addEventListener('change', _empSyncNatType);
  });

  // 管理職 toggle — clicking anywhere in the row (pill or 「管理職」 text)
  // flips aria-checked. Keyboard space/enter on the pill also works.
  _wireEmpManagerToggle('emp-f-is-manager');

  // 內部經歷 — open the add-history sub-modal
  $('#emp-history-add-btn')?.addEventListener('click', openEmpHistoryModal);
  // Edit / view / delete 異動 row (event-delegated so it works for dynamic rows).
  // The 新進 entry can be edited but not deleted — it's the canonical hire record.
  $('#emp-history-tbody')?.addEventListener('click', (e) => {
    const viewBtn = e.target.closest('[data-emp-history-view]');
    if (viewBtn) {
      const tr = viewBtn.closest('tr');
      const idx = parseInt(tr?.dataset.empHistoryIndex || '', 10);
      const sourceRow = _empHistoryViewSourceRows[idx];
      if (sourceRow) openEmpHistoryModal({ row: sourceRow, view: true });
      return;
    }
    const editBtn = e.target.closest('[data-emp-history-edit]');
    if (editBtn) {
      const tr = editBtn.closest('tr');
      if (tr) openEmpHistoryModal({ row: tr });
      return;
    }
    const delBtn = e.target.closest('[data-emp-history-delete]');
    if (!delBtn) return;
    const tr = delBtn.closest('tr');
    const reason = tr?.cells[2]?.textContent.trim();
    if (reason === '新進') {
      showToast?.({ title: '新進紀錄無法刪除', variant: 'warning' });
      return;
    }
    confirmDialog({
      title: '確定刪除此筆異動紀錄？',
      desc: '刪除後將無法復原。',
      confirmText: '確定刪除',
      onConfirm: () => {
        tr?.remove();
        showToast?.({ title: '已刪除異動紀錄', variant: 'success' });
      },
    });
  });
  const historyModal = $('#emp-history-modal');
  $('#emp-history-modal-close')?.addEventListener('click', closeEmpHistoryModal);
  $('#emp-history-modal-cancel')?.addEventListener('click', closeEmpHistoryModal);
  $('#emp-history-modal-save')?.addEventListener('click', _saveEmpHistory);
  historyModal?.addEventListener('click', (e) => {
    if (e.target === historyModal) closeEmpHistoryModal();
  });

  // 管理職 toggle inside the history modal — same row-level flip
  _wireEmpManagerToggle('emp-h-is-manager');

  // View modal — close handlers
  const viewModal = $('#emp-view-modal');
  $('#emp-view-modal-close')?.addEventListener('click', closeEmpViewModal);
  $('#emp-view-modal-cancel')?.addEventListener('click', closeEmpViewModal);
  viewModal?.addEventListener('click', (e) => {
    if (e.target === viewModal) closeEmpViewModal();
  });

  // Initial render of count + pagination
  refreshEmployeeView();
}

// ---------- 內部經歷 add modal ----------
//
// Opens on top of the emp-modal. Adds a row to the history table on save.
// Reason values map to badge variants for visual differentiation in the table.

const _EMP_HISTORY_BADGE = {
  '新進': 'badge-secondary',
  '升遷': 'badge-info',
  '降調': 'badge-warning',
  '轉調': 'badge-secondary',
  '組織調整': 'badge-secondary',
  '復職': 'badge-success',
  '留職停薪': 'badge-warning',
  '離職': 'badge-destructive',
};

// When non-null, _saveEmpHistory will UPDATE this row instead of appending.
let _empHistoryEditingRow = null;

// Transforms the 異動紀錄 modal between editable form and plain-text preview.
// Mirrors _empApplyViewMode's per-field swap but scoped to #emp-history-modal.
function _empApplyHistoryModalViewMode(modal, isView) {
  if (!modal) return;
  modal.classList.toggle('emp-history-view-mode', isView);

  modal.querySelectorAll('.form-field, .emp-radio-row').forEach(field => {
    const wrap = field.querySelector('.input-wrap');
    const toggleRow = field.querySelector('.emp-toggle-row');
    const bareTextarea = [...field.children].find(c => c.tagName === 'TEXTAREA');
    const controls = [wrap, toggleRow, bareTextarea].filter(Boolean);

    if (!isView) {
      controls.forEach(el => { el.style.display = ''; });
      field.style.display = '';
      field.querySelector('.emp-view-text')?.remove();
      return;
    }

    let value = '';
    const select = field.querySelector('select');
    const textarea = field.querySelector('textarea');
    const input = field.querySelector('input:not([type="radio"])');
    const toggleEl = field.querySelector('.toggle');

    if (select) {
      const opt = select.options[select.selectedIndex];
      value = opt ? opt.text : '';
      if (!opt?.value || value === '請選擇' || value.startsWith('—')) value = '';
    } else if (input) {
      value = input.value;
      if (input.type === 'date' && /^\d{4}-\d{2}-\d{2}$/.test(value)) {
        value = value.replaceAll('-', '/');
      }
    } else if (textarea) {
      value = textarea.value;
    } else if (toggleEl) {
      value = toggleEl.getAttribute('aria-checked') === 'true' ? '是' : '否';
    }

    if (!value) { field.style.display = 'none'; return; }
    field.style.display = '';
    controls.forEach(el => { el.style.display = 'none'; });

    let display = field.querySelector('.emp-view-text');
    if (!display) {
      display = document.createElement('div');
      display.className = 'emp-view-text';
      field.appendChild(display);
    }
    display.textContent = value;
  });

  // Collapse empty rows
  modal.querySelectorAll('.form-field-row').forEach(row => {
    if (!isView) { row.style.display = ''; return; }
    const hasVisible = [...row.children].some(c => c.style.display !== 'none' && !c.hidden);
    row.style.display = hasVisible ? '' : 'none';
  });

  // Sub-titles hidden if no following visible content (first title always kept)
  modal.querySelectorAll('.form-section-title').forEach(title => {
    if (!isView) { title.style.display = ''; return; }
    const parent = title.parentElement;
    if (parent?.querySelector('.form-section-title') === title) {
      title.style.display = '';
      return;
    }
    let hasContent = false;
    let next = title.nextElementSibling;
    while (next && !next.classList.contains('form-section-title')) {
      if (next.style.display !== 'none' && !next.hidden) { hasContent = true; break; }
      next = next.nextElementSibling;
    }
    title.style.display = hasContent ? '' : 'none';
  });
}

function openEmpHistoryModal({ row = null, view = false } = {}) {
  const modal = document.getElementById('emp-history-modal');
  if (!modal) return;
  _empHistoryEditingRow = row;

  // Reset all fields
  modal.querySelectorAll('input, textarea').forEach(el => { el.value = ''; });
  modal.querySelectorAll('select').forEach(el => { el.selectedIndex = 0; });
  // Populate 上級 / 代理人 dropdowns from ORG_DATA
  const members = (typeof ORG_DATA !== 'undefined' ? ORG_DATA : []);
  const opts = members.map(m => `<option value="${m.id}">${m.nameZh}(${m.dept || '—'})</option>`).join('');
  ['emp-h-supervisor', 'emp-h-deputy'].forEach(id => {
    const sel = document.getElementById(id);
    if (sel) sel.innerHTML = '<option value="">— 未指派 —</option>' + opts;
  });

  // Field behavior per mode:
  //   view  → plain-text display via _empApplyHistoryModalViewMode (no inputs)
  //   新進 (edit) → only 備註 editable
  //   else  → all editable
  const reasonText = row?.cells[2]?.textContent.trim() || '';
  const newHireMode = !view && row && reasonText === '新進';
  _empApplyHistoryModalViewMode(modal, view);
  if (!view) {
    modal.querySelectorAll('input, textarea, select').forEach(el => {
      el.disabled = newHireMode && el.id !== 'emp-h-note';
    });
  }
  const cancelBtn = document.getElementById('emp-history-modal-cancel');
  if (cancelBtn) cancelBtn.textContent = view ? '關閉' : '取消';

  // Modal title + save button label reflect mode
  const titleEl = document.getElementById('emp-history-modal-title');
  if (titleEl) {
    titleEl.textContent = view
      ? '異動紀錄'
      : newHireMode ? '新進紀錄'
      : row ? '編輯異動紀錄' : '新增異動紀錄';
  }
  const saveBtn = document.getElementById('emp-history-modal-save');
  if (saveBtn) {
    saveBtn.hidden = view;
    saveBtn.textContent = row ? '儲存' : '新增';
  }

  // Edit mode — pre-fill from the row's cells
  if (row) {
    const toIso = (s) => /^\d{4}\/\d{2}\/\d{2}$/.test(s) ? s.replaceAll('/', '-') : '';
    const start = row.cells[0]?.textContent.trim() || '';
    const endTd = row.cells[1];
    const endText = endTd?.textContent.trim() || '';
    const endIsCurrent = endTd?.classList.contains('muted') || endText === '現職';
    const reason = row.cells[2]?.textContent.trim() || '';
    const dept = row.cells[3]?.textContent.trim() || '';
    const title = row.cells[4]?.textContent.trim() || '';
    const idType = row.cells[5]?.textContent.trim() || '';
    const setVal = (id, v) => { const el = document.getElementById(id); if (el) el.value = v; };
    setVal('emp-h-reason', reason);
    setVal('emp-h-start', toIso(start));
    setVal('emp-h-end', endIsCurrent ? '' : toIso(endText));
    setVal('emp-h-dept', dept);
    setVal('emp-h-title', title);
    setVal('emp-h-id-type', idType);
  }

  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  if (window.lucide?.createIcons) lucide.createIcons();
  setTimeout(() => document.getElementById('emp-h-reason')?.focus(), 50);
}

function closeEmpHistoryModal() {
  const modal = document.getElementById('emp-history-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
  _empHistoryEditingRow = null;
}

function _saveEmpHistory() {
  const reason = document.getElementById('emp-h-reason')?.value;
  const start = document.getElementById('emp-h-start')?.value;
  const dept = document.getElementById('emp-h-dept')?.value;
  const title = document.getElementById('emp-h-title')?.value;
  const idType = document.getElementById('emp-h-id-type')?.value;
  if (!reason || !start || !dept || !title || !idType) {
    showToast?.({ title: '請填寫所有必填欄位', variant: 'warning' });
    return;
  }
  const end = document.getElementById('emp-h-end')?.value;
  const tbody = document.getElementById('emp-history-tbody');
  if (!tbody) { closeEmpHistoryModal(); return; }
  const badgeCls = _EMP_HISTORY_BADGE[reason] || 'badge-secondary';
  const fmt = (d) => d ? d.replaceAll('-', '/') : '';
  const deleteBtn = reason === '新進'
    ? `<button class="icon-button" type="button" tabindex="-1" aria-hidden="true" style="visibility:hidden"><i data-lucide="trash-2" class="icon"></i></button>`
    : `<button class="icon-button" type="button" data-emp-history-delete aria-label="刪除"><i data-lucide="trash-2" class="icon"></i></button>`;
  const rowHtml = `
    <td class="p-small">${fmt(start)}</td>
    <td class="p-small${end ? '' : ' muted'}">${end ? fmt(end) : '現職'}</td>
    <td><span class="badge ${badgeCls}">${reason}</span></td>
    <td>${dept}</td>
    <td>${title}</td>
    <td>${idType}</td>
    <td class="emp-history-action-col text-right"><button class="icon-button" type="button" data-emp-history-edit aria-label="編輯"><i data-lucide="pencil" class="icon"></i></button>${deleteBtn}</td>
  `;
  const isEditing = !!_empHistoryEditingRow;
  if (isEditing) {
    _empHistoryEditingRow.innerHTML = rowHtml;
  } else {
    const tr = document.createElement('tr');
    tr.innerHTML = rowHtml;
    tbody.appendChild(tr);
  }
  iconsRefresh();
  closeEmpHistoryModal();
  showToast?.({ title: isEditing ? '已更新異動紀錄' : '已新增異動紀錄', variant: 'success' });
}

// Sync 本國/外國籍 toggle:
//   本國籍 → show .emp-local-only (法規身份), hide .emp-foreign-only
//   外國籍 → hide .emp-local-only, show .emp-foreign-only (外籍員工資料)
//   生理資料 is outside both classes — always visible.
// Wire a 管理職 toggle so the whole .emp-toggle-row (pill + label text) is
// clickable. The toggle pill itself stays keyboard-focusable.
function _wireEmpManagerToggle(toggleId) {
  const toggle = document.getElementById(toggleId);
  if (!toggle) return;
  const row = toggle.closest('.emp-toggle-row') || toggle;
  const flip = () => {
    const next = toggle.getAttribute('aria-checked') !== 'true';
    toggle.setAttribute('aria-checked', next ? 'true' : 'false');
  };
  row.addEventListener('click', () => flip());
  toggle.addEventListener('keydown', (e) => {
    if (e.key === ' ' || e.key === 'Enter') {
      e.preventDefault();
      flip();
    }
  });
}

function _empSyncNatType() {
  const modal = document.getElementById('emp-modal');
  if (!modal) return;
  const isForeign = modal.querySelector('input[name="emp-f-nat-type"]:checked')?.value === 'foreign';
  modal.querySelectorAll('.emp-local-only').forEach(el => { el.hidden = isForeign; });
  modal.querySelectorAll('.emp-foreign-only').forEach(el => { el.hidden = !isForeign; });
}

// Show 畢業日期 when 畢業, 肄業日期 when 肄業, neither when unset
function _empSyncEduStatus(value) {
  const modal = document.getElementById('emp-modal');
  if (!modal) return;
  modal.querySelectorAll('.emp-edu-graduated-only').forEach(el => { el.hidden = value !== 'graduated'; });
  modal.querySelectorAll('.emp-edu-withdrawn-only').forEach(el => { el.hidden = value !== 'withdrawn'; });
}

// Populate the emp-modal 上級 (supervisor) + 代理人 (deputy) dropdowns
// from ORG_DATA. Both sourced from the same member pool; current employee
// (if any) is excluded so they can't be their own supervisor/deputy.
function _empSyncSupervisorOptions(currentId = '', selfEmpId = '') {
  const members = (typeof ORG_DATA !== 'undefined' ? ORG_DATA : []);
  const opts = members
    .filter(m => m.id !== selfEmpId)
    .map(m => `<option value="${m.id}"${currentId === m.id ? ' selected' : ''}>${m.nameZh}(${m.dept || '—'})</option>`)
    .join('');
  ['emp-f-supervisor', 'emp-f-deputy'].forEach(id => {
    const sel = document.getElementById(id);
    if (sel) sel.innerHTML = '<option value="">— 未指派 —</option>' + opts;
  });
}

// Repopulate the emp-modal 職位 <select> based on the selected 部門.
// Includes the currentTitle even if it's not in the dept's position list
// (e.g. edit mode where the existing title was free-text legacy data).
function _empSyncTitleOptions(currentTitle = '') {
  const titleSel = document.getElementById('emp-f-title');
  if (!titleSel) return;
  const dept = document.getElementById('emp-f-dept')?.value || '';
  const positions = [...(_orgPositions?.get(dept) || new Set())].sort((a, b) => a.localeCompare(b));
  if (currentTitle && !positions.includes(currentTitle)) positions.unshift(currentTitle);
  titleSel.innerHTML = '<option value="">— 未指派 —</option>'
    + positions.map(p => `<option value="${p}"${currentTitle === p ? ' selected' : ''}>${p}</option>`).join('');
}

// Edit-mode thead/tbody innerHTML snapshot of the 內部經歷 table, taken when
// switching INTO view mode so we can restore on the way back out.
let _empHistoryEditBackup = null;
// Cloned original rows keyed by view-mode row index — used by the eye-icon
// click to open the history modal pre-filled with the full record.
let _empHistoryViewSourceRows = [];

// Transforms the 內部經歷 table between edit-mode (7 cols incl. delete) and
// view-mode (5 cols: 任職期間 / 異動原因 / 部門+職務 / 身分類別 / 檢視). Idempotent.
function _empApplyHistoryViewMode(modal, isView) {
  const tbody = modal.querySelector('#emp-history-tbody');
  if (!tbody) return;
  const theadRow = tbody.parentElement.querySelector('thead tr');
  if (!theadRow) return;

  // Always restore first if we have a backup (handles re-entry)
  if (_empHistoryEditBackup) {
    theadRow.innerHTML = _empHistoryEditBackup.thead;
    tbody.innerHTML = _empHistoryEditBackup.tbody;
    _empHistoryEditBackup = null;
    _empHistoryViewSourceRows = [];
    iconsRefresh();
  }

  if (!isView) return;

  // Snapshot edit-mode markup, clone source rows for later lookup,
  // then rebuild a 5-column view-mode tbody
  _empHistoryEditBackup = { thead: theadRow.innerHTML, tbody: tbody.innerHTML };
  const rows = [...tbody.querySelectorAll('tr')];
  _empHistoryViewSourceRows = rows.map(r => r.cloneNode(true));

  theadRow.innerHTML = `<th>任職期間</th><th>異動原因</th><th>部門 / 職務</th><th>身分類別</th><th class="emp-history-action-col" aria-label="操作"></th>`;
  tbody.innerHTML = rows.map((r, i) => {
    const start = r.cells[0]?.textContent.trim() || '';
    const endTd = r.cells[1];
    const endText = endTd?.textContent.trim() || '';
    const endIsCurrent = endTd?.classList.contains('muted') || endText === '現職';
    const reasonHtml = r.cells[2]?.innerHTML || '';
    const dept = r.cells[3]?.textContent.trim() || '';
    const title = r.cells[4]?.textContent.trim() || '';
    const idType = r.cells[5]?.textContent.trim() || '';
    const period = endIsCurrent
      ? `${start} - <span class="muted">現職</span>`
      : `${start} - ${endText}`;
    return `<tr data-emp-history-index="${i}">
      <td class="p-small">${period}</td>
      <td>${reasonHtml}</td>
      <td><div>${dept}</div>${title ? `<div class="p-mini muted">${title}</div>` : ''}</td>
      <td>${idType}</td>
      <td class="emp-history-action-col text-right"><button class="icon-button" type="button" data-emp-history-view aria-label="檢視"><i data-lucide="eye" class="icon"></i></button></td>
    </tr>`;
  }).join('');
}

// Section headers shown in view mode (must match the sidebar labels)
const _EMP_VIEW_SECTION_LABELS = {
  basic: '基本資料',
  employment: '在職資料',
  education: '學歷兵役',
  contact: '通訊資料',
  insurance: '保險資料',
  history: '內部經歷',
};

// View-mode renders all sections stacked, fields as plain text, hides empties.
// Restore mode returns the modal to its editable state. Idempotent.
function _empApplyViewMode(modal, isView) {
  // Show all panes in view mode (edit mode is handled by the sidebar logic)
  if (isView) {
    modal.querySelectorAll('[data-emp-pane]').forEach(p => { p.hidden = false; });
  }
  // Swap the 內部經歷 table between edit (7-col) and view (4-col merged)
  _empApplyHistoryViewMode(modal, isView);

  const containers = modal.querySelectorAll(
    '.emp-modal-sections .form-field, .emp-modal-sections .emp-radio-row'
  );
  containers.forEach(field => {
    const wrap = field.querySelector('.input-wrap');
    const radioGroup = field.querySelector('.emp-radio-group');
    const toggleRow = field.querySelector('.emp-toggle-row');
    // Textareas placed directly inside .form-field (no .input-wrap) also need
    // to be hidden in view mode and restored on switch back.
    const bareTextarea = [...field.children].find(c => c.tagName === 'TEXTAREA');
    const controls = [wrap, radioGroup, toggleRow, bareTextarea].filter(Boolean);

    if (!isView) {
      controls.forEach(el => { el.style.display = ''; });
      field.style.display = '';
      field.querySelector('.emp-view-text')?.remove();
      return;
    }

    // Compute the value to render as text
    let value = '';
    const select = field.querySelector('select');
    const textarea = field.querySelector('textarea');
    const input = field.querySelector('input:not([type="radio"])');
    const radios = field.querySelectorAll('input[type="radio"]');
    const toggleEl = field.querySelector('.toggle');

    if (select) {
      const opt = select.options[select.selectedIndex];
      value = opt ? opt.text : '';
      if (!opt?.value || value === '請選擇' || value.startsWith('—')) value = '';
    } else if (input) {
      value = input.value;
      if (input.type === 'date' && /^\d{4}-\d{2}-\d{2}$/.test(value)) {
        value = value.replaceAll('-', '/');
      }
    } else if (textarea) {
      value = textarea.value;
    } else if (radios.length) {
      const checked = [...radios].find(r => r.checked);
      if (checked) value = (checked.parentElement?.textContent || '').trim();
    } else if (toggleEl) {
      value = toggleEl.getAttribute('aria-checked') === 'true' ? '是' : '否';
    }

    // Hide entire field when no value
    if (!value) {
      field.style.display = 'none';
      return;
    }

    field.style.display = '';
    controls.forEach(el => { el.style.display = 'none'; });

    let display = field.querySelector('.emp-view-text');
    if (!display) {
      display = document.createElement('div');
      display.className = 'emp-view-text';
      field.appendChild(display);
    }
    display.textContent = value;
  });

  // Collapse form-field-rows that have no visible children
  modal.querySelectorAll('.emp-modal-sections .form-field-row').forEach(row => {
    if (!isView) { row.style.display = ''; return; }
    const hasVisible = [...row.children].some(c => c.style.display !== 'none' && !c.hidden);
    row.style.display = hasVisible ? '' : 'none';
  });

  // Collapse section subtitles followed by no visible content — but always
  // keep the first title of each pane (the section header).
  // In view mode, rewrite each pane's first title to match the sidebar label.
  modal.querySelectorAll('.emp-modal-sections .form-section-title').forEach(title => {
    if (!isView) {
      title.style.display = '';
      if (title.dataset.viewOriginalText) {
        title.textContent = title.dataset.viewOriginalText;
        delete title.dataset.viewOriginalText;
      }
      return;
    }
    const pane = title.closest('[data-emp-pane]');
    if (pane && pane.querySelector('.form-section-title') === title) {
      const desired = _EMP_VIEW_SECTION_LABELS[pane.dataset.empPane];
      if (desired && title.textContent.trim() !== desired) {
        if (!title.dataset.viewOriginalText) title.dataset.viewOriginalText = title.textContent;
        title.textContent = desired;
      }
      title.style.display = '';
      return;
    }
    let hasContent = false;
    let next = title.nextElementSibling;
    while (next && !next.classList.contains('form-section-title')) {
      if (next.style.display !== 'none' && !next.hidden) { hasContent = true; break; }
      next = next.nextElementSibling;
    }
    title.style.display = hasContent ? '' : 'none';
  });

  // Show 「無」 placeholder for top-level sections with no visible data
  modal.querySelectorAll('[data-emp-pane]').forEach(pane => {
    let placeholder = pane.querySelector(':scope > .emp-view-empty');
    if (!isView) { placeholder?.remove(); return; }
    const hasContent = [...pane.children].some(c => {
      if (c.classList.contains('form-section-title')) return false;
      if (c.classList.contains('emp-view-empty')) return false;
      return c.style.display !== 'none' && !c.hidden;
    });
    if (hasContent) {
      placeholder?.remove();
    } else if (!placeholder) {
      placeholder = document.createElement('div');
      placeholder.className = 'emp-view-empty';
      placeholder.textContent = '無';
      pane.appendChild(placeholder);
    }
  });
}

function openEmpModal(mode, row = null, { section = 'basic' } = {}) {
  const modal = $('#emp-modal');
  if (!modal) return;

  if (mode === 'view') {
    const empName = row?.querySelector('.workspace-member .p-medium')?.textContent.trim();
    $('#emp-modal-title').textContent = empName ? `${empName} · 成員資料` : '成員資料';
  } else {
    $('#emp-modal-title').textContent = mode === 'edit' ? '編輯成員' : '新增成員';
  }

  // Reset sidebar — default to 基本資料, or jump to the requested section.
  // 內部經歷 is hidden in add mode (no history exists for a brand-new member).
  const showHistoryNav = mode !== 'add';
  const histNav = modal.querySelector('[data-emp-section="history"]');
  if (histNav) histNav.hidden = !showHistoryNav;
  modal.querySelectorAll('[data-emp-section]').forEach(b => b.classList.toggle('active', b.dataset.empSection === section));
  modal.querySelectorAll('[data-emp-pane]').forEach(p => { p.hidden = p.dataset.empPane !== section; });

  // Reset 國籍類型 to 本國籍 + sync conditional fields
  const localRadio = modal.querySelector('input[name="emp-f-nat-type"][value="local"]');
  if (localRadio) localRadio.checked = true;
  _empSyncNatType();

  // Reset all fields
  modal.querySelectorAll('input, textarea').forEach(el => {
    if (el.type === 'radio') return;  // preserve nat-type radio reset above
    el.value = ''; el.disabled = false;
  });
  modal.querySelectorAll('select').forEach(el => { el.selectedIndex = 0; el.disabled = false; });

  const hireDate = $('#emp-f-hire-date');
  const resignRow = $('#emp-f-resign-row');
  const leaveRow = $('#emp-f-leave-row');

  // Reset state-specific UI before populating
  if (resignRow) resignRow.hidden = true;
  if (leaveRow) leaveRow.hidden = true;
  _empSyncEduStatus($('#emp-f-edu-status')?.value || '');

  if ((mode === 'edit' || mode === 'view') && row) {
    $('#emp-f-id').value = row.dataset.empId || '';

    // 成員 cell holds name + email (cell index 1 — col 0 is 員工編號)
    const nameEl = row.querySelector('.workspace-member .p-medium');
    if (nameEl) $('#emp-f-name').value = nameEl.textContent.trim();
    const emailEl = row.querySelector('.workspace-member .p-mini');
    if (emailEl) $('#emp-f-email').value = emailEl.textContent.trim();

    const cells = row.cells;
    // 部門/職稱 combined cell: first <div> = 部門, second .p-mini = 職位
    const positionCell = row.querySelector('.emp-position');
    if (positionCell) {
      const deptText = positionCell.children[0]?.textContent.trim();
      const titleText = positionCell.querySelector('.p-mini')?.textContent.trim();
      if (deptText) $('#emp-f-dept').value = deptText;
      // Title is now a <select> populated from the chosen dept's positions
      _empSyncTitleOptions(titleText || '');
    }
    const typeText = cells[4]?.textContent.trim();
    if (typeText) $('#emp-f-type').value = EMP_TYPE_TO_VALUE[typeText] || 'full-time';
    const phoneText = cells[5]?.textContent.trim();
    if (phoneText) $('#emp-f-mobile').value = phoneText;
    // Status — map badge text to dropdown value
    const statusText = row.querySelector('.badge')?.textContent.trim();
    const statusMap = { '在職': 'active', '試用中': 'probation', '留停': 'on-leave', '待加入': 'pending', '離職': 'resigned' };
    const statusVal = statusMap[statusText] || 'active';
    $('#emp-f-status').value = statusVal;
    if (resignRow) resignRow.hidden = statusVal !== 'resigned';
    if (leaveRow) leaveRow.hidden = statusVal !== 'on-leave';

    const hireMatch = cells[7]?.textContent.trim().match(/^(\d{4}\/\d{2}\/\d{2})/);
    if (hireMatch) hireDate.value = hireMatch[1].replaceAll('/', '-');

    hireDate.disabled = true;
  } else {
    // Add mode — auto-generate next 員工編號 (IKL-YYYY-NNNN)
    $('#emp-f-id').value = _empNextEmployeeId();
    // Reset 職位 options for the (empty) initial dept
    _empSyncTitleOptions('');
  }
  // Populate the 上級 dropdown (excludes self in edit mode)
  _empSyncSupervisorOptions('', $('#emp-f-id').value);

  // View-mode tweaks — replace fields with plain text, swap footer buttons
  const isView = mode === 'view';
  modal.classList.toggle('emp-modal-view-mode', isView);
  _empApplyViewMode(modal, isView);
  modal._currentRow = row;  // referenced by 編輯資料 button
  const saveBtn = $('#emp-modal-save');
  if (saveBtn) saveBtn.hidden = isView;
  const cancelBtn = $('#emp-modal-cancel');
  if (cancelBtn) cancelBtn.textContent = isView ? '關閉' : '取消';
  const editFromViewBtn = $('#emp-modal-edit-from-view');
  if (editFromViewBtn) editFromViewBtn.hidden = !isView;
  const histAddBtn = $('#emp-history-add-btn');
  if (histAddBtn) histAddBtn.hidden = isView;

  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  if (window.lucide?.createIcons) lucide.createIcons();
}

function closeEmpModal() {
  const modal = $('#emp-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
}

// ---------- Employee view modal (read-only preview) ----------
//
// Renders a vertical, sectioned summary of the employee row. Reads cells from
// the table row directly so it stays in sync with whatever was last entered
// in the edit modal (mock-data backing).

function _empViewField(label, value, opts = {}) {
  const v = (value || '').trim();
  const isEmpty = !v;
  return `
    <div class="emp-view-field${opts.full ? ' emp-view-field-full' : ''}">
      <div class="emp-view-field-label">${label}</div>
      <div class="emp-view-field-value${isEmpty ? ' empty' : ''}">${isEmpty ? '—' : v}</div>
    </div>
  `;
}

function openEmpViewModal(row) {
  const modal = $('#emp-view-modal');
  const body = $('#emp-view-body');
  const titleEl = $('#emp-view-modal-title');
  if (!modal || !body || !row) return;

  // Pull display values from the row cells (matches the columns rendered in HTML)
  const empId = row.dataset.empId || row.cells[1]?.textContent.trim() || '';
  const nameEl = row.querySelector('.workspace-member .p-medium');
  const enNameEl = row.querySelector('.workspace-member .p-mini');
  const name = nameEl?.textContent.trim() || '';
  const enName = enNameEl?.textContent.trim() || '';
  const positionCell = row.cells[2];
  const dept = positionCell?.children[0]?.textContent.trim() || '';
  const title = positionCell?.querySelector('.p-mini')?.textContent.trim() || '';
  const empType = row.cells[3]?.textContent.trim() || '';
  const phone = row.cells[4]?.textContent.trim() || '';
  const status = row.querySelector('.badge')?.textContent.trim() || '';
  const hireRaw = row.cells[6]?.textContent.trim() || '';
  const hireMatch = hireRaw.match(/^(\d{4}\/\d{2}\/\d{2})/);
  const hireDate = hireMatch ? hireMatch[1] : hireRaw;

  if (titleEl) titleEl.textContent = name ? `${name} · 成員資料` : '成員資料';

  // Sections mirror the sidebar in the edit modal
  body.innerHTML = `
    <div class="emp-view-section">
      <div class="emp-view-section-title">到職資訊</div>
      <div class="emp-view-grid">
        ${_empViewField('員工編號', empId)}
        ${_empViewField('狀態', status)}
        ${_empViewField('部門', dept)}
        ${_empViewField('職位', title)}
        ${_empViewField('員工類別', empType)}
        ${_empViewField('到職日期', hireDate)}
      </div>
    </div>
    <div class="emp-view-section">
      <div class="emp-view-section-title">基本資料</div>
      <div class="emp-view-grid">
        ${_empViewField('姓名', name)}
        ${_empViewField('英文名', enName)}
      </div>
    </div>
    <div class="emp-view-section">
      <div class="emp-view-section-title">聯絡資訊</div>
      <div class="emp-view-grid">
        ${_empViewField('手機', phone)}
      </div>
    </div>
  `;
  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  if (window.lucide?.createIcons) lucide.createIcons();
}

function closeEmpViewModal() {
  const modal = $('#emp-view-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
}

// ---------- Employee bulk import / export ----------
const EMP_CSV_HEADERS = ['員工編號', '姓名', 'Email', '部門', '職位', '類別', '電話', '狀態', '到職日期'];

function _empCsvEscape(v) { return `"${(v ?? '').toString().replace(/"/g, '""')}"`; }

function _triggerDownload(filename, content, mime = 'text/csv;charset=utf-8') {
  // BOM prefix so Excel opens UTF-8 CSV with correct encoding
  const blob = new Blob(['﻿' + content], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

function exportEmployeeCsv() {
  const rows = document.querySelectorAll('.employee-tbody tr');
  const records = [...rows].map(r => {
    const cells = r.cells;
    const positionCell = cells[2];
    const dept = positionCell?.children[0]?.textContent.trim() || '';
    const title = positionCell?.querySelector('.p-mini')?.textContent.trim() || '';
    const hireMatch = cells[6]?.textContent.trim().match(/^(\d{4}\/\d{2}\/\d{2})/);
    return [
      r.dataset.empId || '',
      r.querySelector('.workspace-member .p-medium')?.textContent.trim() || '',
      r.querySelector('.workspace-member .p-mini')?.textContent.trim() || '',
      dept,
      title,
      cells[3]?.textContent.trim() || '',
      cells[4]?.textContent.trim() || '',
      r.querySelector('.badge')?.textContent.trim() || '',
      hireMatch ? hireMatch[1] : '',
    ];
  });
  const csv = [EMP_CSV_HEADERS, ...records]
    .map(row => row.map(_empCsvEscape).join(','))
    .join('\n');
  const dateStr = new Date().toISOString().slice(0, 10);
  _triggerDownload(`員工列表_${dateStr}.csv`, csv);
  showToast?.({ title: `已下載 ${records.length} 筆員工資料`, variant: 'success' });
}

function downloadEmployeeTemplate() {
  const sample = ['', '王小明', 'wang@ikala.ai', '產品開發部', 'Engineer', '全職', '0900-000-000', '待加入', '2026/05/15'];
  const csv = [EMP_CSV_HEADERS, sample]
    .map(row => row.map(_empCsvEscape).join(','))
    .join('\n');
  _triggerDownload('員工匯入範本.csv', csv);
  showToast?.({ title: '範本已下載', variant: 'success' });
}

// Mock parsed-CSV employees shown in the preview step. Used as a deterministic
// pool — number of rows depends on the picked file's size.
const MOCK_IMPORT_PEOPLE = [
  { name: '陳怡君', email: 'yi-chun.chen', dept: '產品開發部', title: 'UI Designer',          type: '全職' },
  { name: '張志明', email: 'zhi-ming.chang', dept: '業務部',     title: 'Account Manager',     type: '全職' },
  { name: '林美華', email: 'mei-hua.lin',   dept: '行銷部',     title: 'Marketing Specialist', type: '兼職' },
  { name: '吳佳穎', email: 'jia-ying.wu',   dept: '客戶成功部', title: 'CS Specialist',        type: '約聘' },
  { name: '黃志偉', email: 'zhi-wei.huang', dept: '管理部',     title: 'HR Specialist',        type: '全職' },
  { name: '王雅婷', email: 'ya-ting.wang',  dept: '產品開發部', title: 'Product Manager',      type: '實習' },
];

let _empImportRows = [];

function _empImportRenderPreview() {
  const list = $('#emp-import-preview-list');
  const confirmBtn = $('#emp-import-confirm');
  const titleEl = $('#emp-import-summary-title');
  if (!list) return;
  if (_empImportRows.length === 0) {
    list.innerHTML = '<div class="emp-import-empty p-small muted">所有資料已被移除,請點「重新上傳」選擇新檔案</div>';
    if (confirmBtn) confirmBtn.disabled = true;
  } else {
    list.innerHTML = _empImportRows.map((r, i) => `
      <div class="emp-import-preview-item" data-idx="${i}">
        <span class="avatar">${r.name[0] || '?'}</span>
        <div class="emp-import-preview-info">
          <div class="p-medium">${r.name}<span class="muted-inline">  ·  ${r.email}@ikala.ai</span></div>
          <div class="p-mini muted">${r.dept} / ${r.title} / ${r.type}</div>
        </div>
        <button class="icon-button" type="button" data-emp-import-remove aria-label="移除">
          <i data-lucide="x" class="icon"></i>
        </button>
      </div>
    `).join('');
    if (confirmBtn) confirmBtn.disabled = false;
  }
  if (titleEl) titleEl.textContent = `已解析 ${_empImportRows.length} 筆員工資料`;
  if (window.lucide?.createIcons) lucide.createIcons();
}

function _empImportSetStep(step) {
  document.querySelectorAll('[data-emp-import-step]').forEach(el => {
    el.hidden = el.dataset.empImportStep !== step;
  });
  document.querySelectorAll('[data-emp-import-on]').forEach(el => {
    el.hidden = el.dataset.empImportOn !== step;
  });
  if (window.lucide?.createIcons) lucide.createIcons();
}

function openEmpImportModal() {
  const modal = $('#emp-import-modal');
  if (!modal) return;
  // Reset upload-step state
  $('#emp-import-file').value = '';
  $('#emp-import-dropmsg').textContent = '點擊或拖曳檔案至此';
  $('#emp-import-next').disabled = true;
  $('#emp-import-drop').classList.remove('selected');
  _empImportRows = [];
  _empImportSetStep('upload');
  modal.classList.add('open');
  modal.setAttribute('aria-hidden', 'false');
  if (window.lucide?.createIcons) lucide.createIcons();
}

function closeEmpImportModal() {
  const modal = $('#emp-import-modal');
  if (!modal) return;
  modal.classList.remove('open');
  modal.setAttribute('aria-hidden', 'true');
}

function initEmpImportModal() {
  const modal = $('#emp-import-modal');
  if (!modal) return;
  $('#emp-import-close')?.addEventListener('click', closeEmpImportModal);
  $('#emp-import-cancel')?.addEventListener('click', closeEmpImportModal);
  $('#emp-import-template')?.addEventListener('click', downloadEmployeeTemplate);
  modal.addEventListener('click', (e) => { if (e.target === modal) closeEmpImportModal(); });

  const drop = $('#emp-import-drop');
  const fileInput = $('#emp-import-file');
  const msg = $('#emp-import-dropmsg');
  const nextBtn = $('#emp-import-next');

  // Native file pick
  fileInput?.addEventListener('change', () => {
    const f = fileInput.files?.[0];
    if (f) {
      msg.textContent = `${f.name}  ·  ${(f.size / 1024).toFixed(1)} KB`;
      drop.classList.add('selected');
      nextBtn.disabled = false;
    }
  });

  // Drag & drop
  ['dragenter', 'dragover'].forEach(evt => {
    drop?.addEventListener(evt, (e) => { e.preventDefault(); drop.classList.add('dragging'); });
  });
  ['dragleave', 'drop'].forEach(evt => {
    drop?.addEventListener(evt, (e) => { e.preventDefault(); drop.classList.remove('dragging'); });
  });
  drop?.addEventListener('drop', (e) => {
    const f = e.dataTransfer?.files?.[0];
    if (f) {
      const dt = new DataTransfer();
      dt.items.add(f);
      fileInput.files = dt.files;
      fileInput.dispatchEvent(new Event('change'));
    }
  });

  // Step 1 → Step 2 (parse + preview)
  nextBtn?.addEventListener('click', () => {
    const f = fileInput.files?.[0];
    if (!f) return;
    // Mock parse: produce 3-6 rows from MOCK_IMPORT_PEOPLE based on file size
    const count = 3 + (Math.floor(f.size / 100) % 4);
    _empImportRows = MOCK_IMPORT_PEOPLE.slice(0, count).map(r => ({ ...r }));
    _empImportRenderPreview();
    _empImportSetStep('preview');
  });

  // Back to Step 1
  $('#emp-import-back')?.addEventListener('click', () => {
    _empImportSetStep('upload');
  });

  // Per-row remove (delegated)
  $('#emp-import-preview-list')?.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-emp-import-remove]');
    if (!btn) return;
    const item = btn.closest('.emp-import-preview-item');
    const idx = parseInt(item.dataset.idx, 10);
    if (!Number.isNaN(idx)) {
      _empImportRows.splice(idx, 1);
      _empImportRenderPreview();
    }
  });

  // Final confirm
  $('#emp-import-confirm')?.addEventListener('click', () => {
    const n = _empImportRows.length;
    if (n === 0) return;
    closeEmpImportModal();
    showToast?.({
      title: `已匯入 ${n} 筆員工資料`,
      desc: '系統正在背景驗證資料,稍後可至「員工列表」查看結果',
      variant: 'success'
    });
  });
}

// Close any open emp-row-menu when clicking outside
document.addEventListener('click', () => {
  document.querySelectorAll('.emp-row-menu.open').forEach(dd => dd.classList.remove('open'));
});

// ---------- Settings modal (個人資料 + 自訂外觀 + 通知與提醒 + 密碼設定) ----------
function initSettingsModal() {
  const modal = $('#settings-modal');
  if (!modal) return;

  const navItems = modal.querySelectorAll('[data-settings-section]');
  const sections = modal.querySelectorAll('.settings-modal-section');
  const closeBtn = $('#settings-modal-close');
  const securitySection = modal.querySelector('[data-section="security"]');
  const navEl = modal.querySelector('.settings-modal-nav');
  const navWrap = modal.querySelector('.settings-modal-nav-wrap');

  // Mobile: show edge chevrons when the tab row still has content to
  // scroll into on that side.
  function updateNavScrollHint() {
    if (!navEl || !navWrap) return;
    const canScrollRight = navEl.scrollLeft + navEl.clientWidth < navEl.scrollWidth - 1;
    const canScrollLeft = navEl.scrollLeft > 0;
    navWrap.classList.toggle('can-scroll-right', canScrollRight);
    navWrap.classList.toggle('can-scroll-left', canScrollLeft);
  }
  navEl?.addEventListener('scroll', updateNavScrollHint, { passive: true });
  window.addEventListener('resize', updateNavScrollHint);

  // Clicking the edge chevrons scrolls the nav row toward that direction.
  if (navWrap && navEl) {
    navWrap.querySelectorAll('[data-nav-scroll]').forEach(btn => {
      btn.addEventListener('click', () => {
        const dir = btn.dataset.navScroll === 'left' ? -1 : 1;
        const amount = Math.round(navEl.clientWidth * 0.7);
        navEl.scrollBy({ left: dir * amount, behavior: 'smooth' });
      });
    });
  }

  function showSection(name) {
    navItems.forEach(i => i.classList.toggle('active', i.dataset.settingsSection === name));
    sections.forEach(s => { s.hidden = s.dataset.section !== name; });
    // Always reset security sub-state back to list when switching sections
    if (name !== 'security') setPasswordState('list');
  }

  function setPasswordState(state) {
    if (!securitySection) return;
    securitySection.querySelectorAll('[data-password-state]').forEach(el => {
      el.hidden = el.dataset.passwordState !== state;
    });
    // Reset fields + strength when leaving the form
    if (state === 'list') {
      ['pw-current', 'pw-new', 'pw-confirm'].forEach(id => {
        const i = $('#' + id);
        if (i) i.value = '';
      });
      const bar = $('#pw-new-strength'); if (bar) bar.className = 'pw-strength';
      const hint = $('#pw-new-hint'); if (hint) hint.textContent = '請輸入新密碼';
      const cHint = $('#pw-confirm-hint'); if (cHint) { cHint.textContent = ''; cHint.className = 'pw-hint'; }
    } else {
      setTimeout(() => $('#pw-current')?.focus(), 60);
    }
    modal.querySelector('.settings-modal-main').scrollTop = 0;
    if (window.lucide?.createIcons) lucide.createIcons();
  }

  function openModal(section = 'profile') {
    showSection(section);
    modal.querySelector('.settings-modal-main').scrollTop = 0;
    modal.classList.add('open');
    modal.setAttribute('aria-hidden', 'false');
    $('#user-dropdown')?.classList.remove('open');
    if (window.lucide?.createIcons) lucide.createIcons();
    // Compute nav overflow after layout settles
    requestAnimationFrame(updateNavScrollHint);
  }
  function closeModal() {
    setPasswordState('list');
    modal.classList.remove('open');
    modal.setAttribute('aria-hidden', 'true');
  }

  // Nav item clicks — switch section
  navItems.forEach(item => {
    item.addEventListener('click', () => showSection(item.dataset.settingsSection));
  });

  // Avatar dropdown items open the modal at a specific section
  document.querySelectorAll('[data-settings-open]').forEach(trigger => {
    trigger.addEventListener('click', (e) => {
      e.stopPropagation();
      openModal(trigger.dataset.settingsOpen);
    });
  });

  closeBtn?.addEventListener('click', closeModal);
  modal.querySelectorAll('[data-settings-close]').forEach(btn => {
    btn.addEventListener('click', closeModal);
  });

  // Password state switches (變更密碼 ↔ 返回)
  modal.addEventListener('click', (e) => {
    const t = e.target.closest('[data-password-action]');
    if (!t) return;
    const action = t.dataset.passwordAction;
    if (action === 'open-change') setPasswordState('change');
    else if (action === 'open-list') setPasswordState('list');
  });

  // Password visibility toggle (shared [data-pw-toggle] — works inside modal too)
  modal.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-pw-toggle]');
    if (!btn) return;
    const input = btn.parentElement.querySelector('input');
    const icon = btn.querySelector('.icon');
    if (!input || !icon) return;
    const show = input.type === 'password';
    input.type = show ? 'text' : 'password';
    icon.setAttribute('data-lucide', show ? 'eye-off' : 'eye');
    btn.setAttribute('aria-label', show ? '隱藏密碼' : '顯示密碼');
    if (window.lucide?.createIcons) lucide.createIcons();
  });

  // New password strength meter
  const pwNew = $('#pw-new');
  if (pwNew) pwNew.addEventListener('input', () => {
    const v = pwNew.value;
    let score = 0;
    if (v.length >= 8) score++;
    if (/[A-Z]/.test(v) && /[a-z]/.test(v)) score++;
    if (/\d/.test(v)) score++;
    if (/[^A-Za-z0-9]/.test(v)) score++;
    const bar = $('#pw-new-strength');
    const hint = $('#pw-new-hint');
    if (bar) bar.className = 'pw-strength' + (v ? ' s' + score : '');
    const labels = ['請輸入新密碼', '密碼太弱', '密碼強度：中等', '密碼強度：良好', '密碼強度：很強'];
    if (hint) hint.textContent = labels[v ? score : 0] || '';
    // Refresh confirm match hint live
    updateConfirmHint();
  });

  const pwConfirm = $('#pw-confirm');
  function updateConfirmHint() {
    const cHint = $('#pw-confirm-hint');
    if (!cHint || !pwConfirm) return;
    const c = pwConfirm.value;
    if (!c) { cHint.textContent = ''; cHint.className = 'pw-hint'; return; }
    if (c === (pwNew?.value || '')) {
      cHint.textContent = '✓ 密碼一致';
      cHint.className = 'pw-hint pw-hint-ok';
    } else {
      cHint.textContent = '密碼不符，請重新輸入';
      cHint.className = 'pw-hint pw-hint-err';
    }
  }
  pwConfirm?.addEventListener('input', updateConfirmHint);

  // Submit — simulate + toast + back to list
  $('#pw-change-submit')?.addEventListener('click', (e) => {
    e.preventDefault();
    const cur = $('#pw-current')?.value || '';
    const nu = pwNew?.value || '';
    const cf = pwConfirm?.value || '';
    if (!cur || !nu || !cf) return;
    if (nu !== cf) { updateConfirmHint(); return; }
    setPasswordState('list');
    if (typeof showToast === 'function') showToast({ title: '密碼已變更', variant: 'success' });
  });

  // Close on overlay click
  modal.addEventListener('click', (e) => {
    if (e.target === modal) closeModal();
  });

  // Esc closes (or goes back from change-password sub-state first)
  document.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape' || !modal.classList.contains('open')) return;
    const inChange = securitySection?.querySelector('[data-password-state="change"]')?.hidden === false;
    if (inChange) setPasswordState('list');
    else closeModal();
  });
}

// ---------- Theme switcher (preferences "外觀" — 淺色/深色/跟隨系統) ----------
function initThemeSwitcher() {
  const THEMES = ['light', 'dark', 'auto'];
  const saved = (() => {
    try { return localStorage.getItem('nexus-theme') || 'light'; } catch { return 'light'; }
  })();

  // Sync initial radio + card state to saved theme
  document.querySelectorAll('.theme-card input[name="pref-theme"]').forEach(input => {
    const match = input.value === saved;
    input.checked = match;
    input.closest('.theme-card')?.classList.toggle('selected', match);
  });

  function applyTheme(theme) {
    if (!THEMES.includes(theme)) theme = 'light';
    document.documentElement.setAttribute('data-theme', theme);
    try { localStorage.setItem('nexus-theme', theme); } catch {}
  }

  document.querySelectorAll('.theme-card input[name="pref-theme"]').forEach(input => {
    input.addEventListener('change', () => {
      document.querySelectorAll('.theme-card input[name="pref-theme"]').forEach(i => {
        i.closest('.theme-card')?.classList.toggle('selected', i.checked);
      });
      if (input.checked) applyTheme(input.value);
    });
  });

  // Refresh system-dependent rendering when OS theme changes in "auto" mode
  if (window.matchMedia) {
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const onChange = () => {
      if (document.documentElement.getAttribute('data-theme') === 'auto') {
        // Nudge any listeners / force repaint of lucide icons (color inherits via currentColor)
        if (window.lucide?.createIcons) lucide.createIcons();
      }
    };
    mq.addEventListener?.('change', onChange);
  }
}

// ---------- Quick prompts: horizontal scroll with left/right arrows ----------
function updateQuickPromptArrows(wrap) {
  const scroller = wrap.querySelector('.quick-prompts');
  const prev = wrap.querySelector('.quick-prompts-prev');
  const next = wrap.querySelector('.quick-prompts-next');
  if (!scroller) return;

  const canScroll = scroller.scrollWidth - scroller.clientWidth > 1;
  const atStart = scroller.scrollLeft <= 1;
  const atEnd = Math.ceil(scroller.scrollLeft + scroller.clientWidth) >= scroller.scrollWidth - 1;

  // If content fits in view, hide both arrows
  if (!canScroll) {
    if (prev) prev.hidden = true;
    if (next) next.hidden = true;
    return;
  }
  if (prev) prev.hidden = atStart;
  if (next) next.hidden = atEnd;
}

function initQuickPrompts() {
  $$('.quick-prompts-wrap').forEach(wrap => {
    const scroller = wrap.querySelector('.quick-prompts');
    const prev = wrap.querySelector('.quick-prompts-prev');
    const next = wrap.querySelector('.quick-prompts-next');
    if (!scroller) return;

    // Click handlers
    prev?.addEventListener('click', () => {
      scroller.scrollBy({ left: -scroller.clientWidth * 0.6, behavior: 'smooth' });
    });
    next?.addEventListener('click', () => {
      scroller.scrollBy({ left: scroller.clientWidth * 0.6, behavior: 'smooth' });
    });

    // Update arrow visibility on scroll
    scroller.addEventListener('scroll', () => updateQuickPromptArrows(wrap));

    // Update on resize of the scroller (ResizeObserver)
    if (typeof ResizeObserver !== 'undefined') {
      const ro = new ResizeObserver(() => updateQuickPromptArrows(wrap));
      ro.observe(scroller);
    } else {
      window.addEventListener('resize', () => updateQuickPromptArrows(wrap));
    }

    // Initial state
    updateQuickPromptArrows(wrap);
  });
}

// ---------- Auto-grow textareas ----------
function initTextareas() {
  // Compute max height from each textarea's own computed font metrics so
  // the welcome-input (16px/1.6) and regular chat-input (15px/1.6) each
  // cap at 5 lines correctly.
  document.addEventListener('input', (e) => {
    const t = e.target;
    if (t.tagName === 'TEXTAREA' && t.closest('.chat-input-wrap')) {
      const cs = getComputedStyle(t);
      const fs = parseFloat(cs.fontSize) || 15;
      const lh = parseFloat(cs.lineHeight) || fs * 1.6;
      const maxH = Math.round(lh * 5);
      t.style.height = 'auto';
      t.style.height = Math.min(maxH, t.scrollHeight) + 'px';
    }
  });
}

// ---------- Home clock widget ----------
let clockedOut = false;

function initHomeClock() {
  const dateEl = $('#home-clock-date');
  const timeEl = $('#home-clock-time');
  if (!dateEl || !timeEl) return;

  const weekdays = ['日', '一', '二', '三', '四', '五', '六'];

  const taskDateEl = $('#task-clock-date');
  const taskTimeEl = $('#task-clock-time');

  function tick() {
    const now = new Date();
    const dateStr = `${now.getFullYear()}/${now.getMonth() + 1}/${now.getDate()} 週${weekdays[now.getDay()]}`;
    const hh = String(now.getHours()).padStart(2, '0');
    const mm = String(now.getMinutes()).padStart(2, '0');
    const ss = String(now.getSeconds()).padStart(2, '0');
    const timeStr = `${hh}:${mm}:${ss}`;
    dateEl.textContent = dateStr;
    timeEl.textContent = timeStr;
    if (taskDateEl) taskDateEl.textContent = dateStr;
    if (taskTimeEl) taskTimeEl.textContent = timeStr;
  }
  tick();
  let clockTimer = null;
  const startTimer = () => { if (!clockTimer) clockTimer = setInterval(tick, 1000); };
  const stopTimer = () => { if (clockTimer) { clearInterval(clockTimer); clockTimer = null; } };
  if (document.visibilityState === 'visible') startTimer();
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') { tick(); startTimer(); } else stopTimer();
  });

  $$('.clock-out-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      if (clockedOut) return;
      const now = new Date();
      const hh = String(now.getHours()).padStart(2, '0');
      const mm = String(now.getMinutes()).padStart(2, '0');
      applyClockOutState(`${hh}:${mm}`);
    });
  });
}

function applyClockOutState(timeStr) {
  const wasFirstTime = !clockedOut;
  clockedOut = true;
  $$('.clock-out-btn').forEach(btn => {
    btn.textContent = '下班打卡 ✓';
    btn.disabled = true;
    btn.classList.remove('btn-primary');
    btn.classList.add('btn-outline');
  });
  $$('.clock-out-status').forEach(el => {
    el.textContent = '已下班';
    el.classList.remove('badge-secondary');
    el.classList.add('badge-success');
  });
  $$('.clock-out-time').forEach(el => {
    el.textContent = timeStr;
    el.hidden = false;
  });

  // Warm send-off — only when the user actually clocks out (not on
  // programmatic restore). Message varies with the time of day.
  if (wasFirstTime && typeof showToast === 'function') {
    const hour = new Date().getHours();
    const msg =
      hour < 17 ? { title: `早退囉，路上小心 👋`, desc: `於 ${timeStr} 下班` }
      : hour < 19 ? { title: `辛苦了，明天見 👋`, desc: `於 ${timeStr} 下班` }
      : hour < 22 ? { title: `今天也加班了，早點休息 🌙`, desc: `於 ${timeStr} 下班` }
      : { title: `這麼晚才下班，辛苦了 🌙`, desc: `於 ${timeStr} 下班` };
    showToast({ ...msg, variant: 'success' });
  }
}

// ---------- Attendance calendar ----------
// Mock attendance data keyed by "YYYY-M"
const ATTENDANCE_DATA = {
  '2026-4': {
    1:'attended',2:'attended',3:'attended',4:'attended',
    7:'attended',8:'attended',9:'leave',10:'attended',11:'attended',
    14:'attended',15:'attended',16:'today',
    // 未打卡 (missed) — past weekdays with no record
    // auto-computed below
  },
  '2026-3': {
    2:'attended',3:'attended',4:'attended',5:'attended',6:'attended',
    9:'attended',10:'attended',11:'attended',12:'attended',13:'attended',
    16:'attended',17:'leave',18:'attended',19:'attended',20:'attended',
    23:'attended',24:'missed',25:'attended',26:'attended',27:'attended',
    30:'attended',31:'attended',
  },
};

// Punch-in records keyed by "YYYY/M/D"
const PUNCH_RECORDS = {
  '2026/4/16': { in: '09:02', out: null,    hours: null,  status: 'working' },
  '2026/4/15': { in: '08:55', out: '18:12', hours: '8.3', status: 'normal' },
  '2026/4/14': { in: '09:10', out: '18:45', hours: '8.6', status: 'normal' },
  '2026/4/13': { in: null,    out: null,     hours: null,  status: 'weekend' },
  '2026/4/12': { in: null,    out: null,     hours: null,  status: 'weekend' },
  '2026/4/11': { in: '09:01', out: '21:05', hours: '11.1', status: 'overtime' },
  '2026/4/10': { in: '08:48', out: '18:30', hours: '8.7', status: 'normal' },
  '2026/4/9':  { in: null,    out: null,     hours: null,  status: 'leave' },
  '2026/4/8':  { in: '09:05', out: '18:20', hours: '8.3', status: 'normal' },
  '2026/4/7':  { in: '08:50', out: '18:15', hours: '8.4', status: 'normal' },
  '2026/4/6':  { in: null,    out: null,     hours: null,  status: 'weekend' },
  '2026/4/5':  { in: null,    out: null,     hours: null,  status: 'weekend' },
  '2026/4/4':  { in: '09:12', out: '18:40', hours: '8.5', status: 'normal' },
  '2026/4/3':  { in: '08:58', out: '18:10', hours: '8.2', status: 'normal' },
  '2026/4/2':  { in: '09:00', out: '18:30', hours: '8.5', status: 'normal' },
  '2026/4/1':  { in: '08:45', out: '18:25', hours: '8.7', status: 'normal' },
  '2026/3/31': { in: '09:02', out: '18:08', hours: '8.1', status: 'normal' },
  '2026/3/30': { in: null,    out: null,     hours: null,  status: 'weekend' },
  '2026/3/29': { in: null,    out: null,     hours: null,  status: 'weekend' },
  '2026/3/28': { in: '08:50', out: '18:15', hours: '8.4', status: 'normal' },
  '2026/3/27': { in: '09:10', out: '18:30', hours: '8.3', status: 'normal' },
  '2026/3/26': { in: '09:05', out: '18:20', hours: '8.3', status: 'normal' },
  '2026/3/25': { in: '08:55', out: '18:00', hours: '8.1', status: 'normal' },
  '2026/3/24': { in: null,    out: null,     hours: null,  status: 'missed' },
  '2026/3/23': { in: null,    out: null,     hours: null,  status: 'weekend' },
  '2026/3/22': { in: null,    out: null,     hours: null,  status: 'weekend' },
};

let currentWeekStart; // Date object — Sunday of the displayed week

function getWeekStart(date) {
  const d = new Date(date);
  d.setDate(d.getDate() - d.getDay()); // Go to Sunday
  d.setHours(0, 0, 0, 0);
  return d;
}

function renderWeekPunch(weekStart) {
  if (!weekStart) weekStart = getWeekStart(new Date());
  currentWeekStart = weekStart;

  const tbody = $('#week-punch-body');
  const titleEl = $('#week-title');
  if (!tbody) return;

  const now = new Date();
  now.setHours(0, 0, 0, 0);
  const dayNames = ['日','一','二','三','四','五','六'];
  const statusMap = {
    working:  { badge: 'badge-info',        text: '工作中' },
    normal:   { badge: 'badge-success',     text: '正常' },
    overtime: { badge: 'badge-warning',     text: '加班' },
    leave:    { badge: 'badge-ghost',       text: '請假' },
    weekend:  { badge: 'badge-secondary',   text: '假日' },
    missed:   { badge: 'badge-destructive', text: '需補卡' },
    future:   { badge: 'badge-secondary',   text: '—' },
  };

  // Determine week range label
  const weekEnd = new Date(weekStart);
  weekEnd.setDate(weekEnd.getDate() + 6);
  const fmt = d => `${d.getMonth() + 1}/${d.getDate()}`;
  const thisWeekStart = getWeekStart(now);
  let label;
  if (weekStart.getTime() === thisWeekStart.getTime()) {
    label = `本週（${fmt(weekStart)}–${fmt(weekEnd)}）`;
  } else {
    label = `${fmt(weekStart)}–${fmt(weekEnd)}`;
  }
  if (titleEl) titleEl.textContent = label;

  // Disable next if already on current week
  const nextBtn = $('#week-next');
  if (nextBtn) nextBtn.disabled = (weekStart.getTime() >= thisWeekStart.getTime());

  // Render 7 rows (Sun–Sat)
  let html = '';
  for (let i = 0; i < 7; i++) {
    const d = new Date(weekStart);
    d.setDate(d.getDate() + i);
    const key = `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()}`;
    const dow = d.getDay();
    const isWeekend = (dow === 0 || dow === 6);
    const isFuture = d > now;

    let rec = PUNCH_RECORDS[key];
    if (!rec) {
      if (isFuture) rec = { in: null, out: null, hours: null, status: 'future' };
      else if (isWeekend) rec = { in: null, out: null, hours: null, status: 'weekend' };
      else rec = { in: null, out: null, hours: null, status: 'missed' };
    }

    const st = statusMap[rec.status] || statusMap.future;
    const dateLabel = `${d.getFullYear()}/${String(d.getMonth() + 1).padStart(2,'0')}/${String(d.getDate()).padStart(2,'0')}（${dayNames[dow]}）`;

    const isHoliday = isWeekend || rec.status === 'weekend';
    html += `<tr class="${isHoliday ? 'dash-row-weekend' : ''}">
      <td>${dateLabel}</td>
      <td>${rec.in || '—'}</td>
      <td>${rec.out || '—'}</td>
      <td>${rec.hours ? rec.hours + 'h' : '—'}</td>
      <td><span class="badge ${st.badge}">${st.text}</span></td>
      <td>${rec.status === 'missed' ? '<a class="link-btn p-small">填寫</a>' : ''}</td>
    </tr>`;
  }
  tbody.innerHTML = html;
}

function initWeekNav() {
  $('#week-prev')?.addEventListener('click', () => {
    const prev = new Date(currentWeekStart);
    prev.setDate(prev.getDate() - 7);
    renderWeekPunch(prev);
  });
  $('#week-next')?.addEventListener('click', () => {
    const thisWeekStart = getWeekStart(new Date());
    if (currentWeekStart.getTime() >= thisWeekStart.getTime()) return;
    const next = new Date(currentWeekStart);
    next.setDate(next.getDate() + 7);
    renderWeekPunch(next);
  });
}

let calendarYear, calendarMonth;

function renderAttendanceCalendar(year, month) {
  const root = $('#dash-calendar');
  const titleEl = $('#cal-title');
  if (!root) return;

  const now = new Date();
  if (year == null) year = now.getFullYear();
  if (month == null) month = now.getMonth();
  calendarYear = year;
  calendarMonth = month;

  const isCurrentMonth = (year === now.getFullYear() && month === now.getMonth());
  const today = isCurrentMonth ? now.getDate() : -1;
  const firstDay = new Date(year, month, 1).getDay();
  const daysInMonth = new Date(year, month + 1, 0).getDate();
  const monthNames = ['1 月','2 月','3 月','4 月','5 月','6 月','7 月','8 月','9 月','10 月','11 月','12 月'];

  if (titleEl) titleEl.textContent = `${monthNames[month]} ${year}`;

  // Get attendance for this month
  const key = `${year}-${month + 1}`;
  const attendance = ATTENDANCE_DATA[key] || {};

  // Weekends
  const weekends = new Set();
  for (let d = 1; d <= daysInMonth; d++) {
    const dow = new Date(year, month, d).getDay();
    if (dow === 0 || dow === 6) weekends.add(d);
  }

  const heads = ['日','一','二','三','四','五','六'];
  let html = heads.map(h => `<div class="dash-cal-head">${h}</div>`).join('');

  for (let i = 0; i < firstDay; i++) html += `<div class="dash-cal-day empty"></div>`;

  for (let d = 1; d <= daysInMonth; d++) {
    let cls = 'dash-cal-day';
    const isWeekend = weekends.has(d);
    const isPast = new Date(year, month, d) < new Date(now.getFullYear(), now.getMonth(), now.getDate());

    if (d === today) {
      cls += ' today';
    } else if (attendance[d] === 'leave') {
      cls += ' leave';
    } else if (attendance[d] === 'missed') {
      cls += ' missed';
    } else if (attendance[d] === 'attended') {
      cls += ' attended';
    } else if (isWeekend) {
      cls += ' weekend';
    } else if (isPast && !isWeekend) {
      // Past weekday with no record → need to punch in (missed)
      cls += ' missed';
    } else {
      cls += ' future';
    }

    html += `<div class="${cls}">${d}</div>`;
  }
  root.innerHTML = html;

  // Disable "next" if already on current month
  const nextBtn = $('#cal-next');
  if (nextBtn) nextBtn.disabled = isCurrentMonth;
}

function renderDashChat() {
  const body = $('#dash-chat-body');
  if (!body) return;
  body.innerHTML = `
    <div class="chat-message">
      <div class="chat-avatar-sm">🤖</div>
      <div class="chat-bubble">
        早安 Tammy！以下是今日摘要：<br><br>
        <b>出勤</b>：你已於 09:02 打卡上班，本月累計出席 12 天、請假 1 天。<br><br>
        <b>業績</b>：個人本月業績 NT$860K，達成率 72%（目標 NT$1,200K），較上月增加 8.1%。<br><br>
        <b>財務</b>：公司本月淨利 NT$4,130K，較上月成長 7.6%。你的部門支出正常。<br><br>
        有什麼想進一步了解的嗎？
      </div>
    </div>
  `;
}

function renderAssistantsChat() {
  const body = $('#assistants-chat-body');
  if (!body) return;
  body.innerHTML = `
    <div class="chat-message">
      <div class="chat-avatar-sm">🤖</div>
      <div class="chat-bubble">
        哈囉 Tammy！告訴我你想處理的事情，我能幫你挑選最合適的助理。<br><br>
        常見需求：<br>
        • 請假 / 加班 / 出差申請<br>
        • 報表分析、OKR 拆解<br>
        • 面試邀約、合約審閱
      </div>
    </div>
    <div class="chat-message user">
      <div class="chat-avatar-sm user">T</div>
      <div class="chat-bubble">我想整理這週客戶回饋</div>
    </div>
    <div class="chat-message">
      <div class="chat-avatar-sm">🤖</div>
      <div class="chat-bubble">
        推薦你使用「<b>客戶關係維繫專家</b>」：可自動彙整 CRM 系統中的客戶回饋，並給出跟進建議。要我替你開啟嗎？
      </div>
    </div>
  `;
}

function renderTaskChat() {
  const body = $('#task-chat-body');
  if (!body) return;
  body.innerHTML = `
    <div class="chat-message">
      <div class="chat-avatar-sm">🤖</div>
      <div class="chat-bubble">
        Hi Tammy！以下是今日工時概況：<br><br>
        <b>本日累計</b>：7h / 7h（已達標）<br>
        <b>本月平均</b>：6.8h / 天<br>
        <b>主要類別</b>：UI/UX Design（42%）、Meeting（21%）<br><br>
        想了解哪一項？我也能幫你快速新增明日的任務。
      </div>
    </div>
    <div class="chat-message user">
      <div class="chat-avatar-sm user">T</div>
      <div class="chat-bubble">本週 Nexus 產品我花了多少時間？</div>
    </div>
    <div class="chat-message">
      <div class="chat-avatar-sm">🤖</div>
      <div class="chat-bubble">
        本週（4/14–4/18）你在 <b>Nexus</b> 累計 <b>14.5h</b>，主要集中在 UI/UX Design（9h）與 Development（3h）。需要我幫你整理成週報嗎？
      </div>
    </div>
  `;
}

function initAttendanceModal() {
  const modal = $('#attendance-modal');
  const closeBtn = $('#attendance-modal-close');
  if (!modal) return;

  $$('.attendance-modal-trigger').forEach(trigger => {
    trigger.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      modal.classList.add('open');
      modal.setAttribute('aria-hidden', 'false');
      updateAttendanceMonth();
      iconsRefresh();
    });
  });

  if (closeBtn) closeBtn.addEventListener('click', () => {
    modal.classList.remove('open');
    modal.setAttribute('aria-hidden', 'true');
  });

  modal.addEventListener('click', (e) => {
    if (e.target === modal) {
      modal.classList.remove('open');
      modal.setAttribute('aria-hidden', 'true');
    }
  });
}

function initCalendarNav() {
  $('#cal-prev')?.addEventListener('click', () => {
    renderAttendanceCalendar(calendarYear, calendarMonth - 1);
    iconsRefresh();
  });
  $('#cal-next')?.addEventListener('click', () => {
    const now = new Date();
    // Don't go past current month
    if (calendarYear === now.getFullYear() && calendarMonth >= now.getMonth()) return;
    renderAttendanceCalendar(calendarYear, calendarMonth + 1);
    iconsRefresh();
  });
}

// ---------- Boot ----------
document.addEventListener('DOMContentLoaded', () => {
  renderAssistantScroller();
  renderFormsGrid();
  renderNotifications();
  renderAIReview();
  renderFormCategories();
  renderDrafts();
  renderAppliedForms();
  renderFormChat();
  renderAssistantsGrid();
  // Assistants search input
  const assistantSearchInput = $('.assistant-search input');
  if (assistantSearchInput) {
    assistantSearchInput.addEventListener('input', (e) => {
      assistantsSearchQuery = e.target.value || '';
      renderAssistantsGrid();
      iconsRefresh();
    });
  }
  renderHistoryChat();

  initTabs();
  initSidebar();
  initDropdownsA11y();
  initDropdown();
  initOverlayKeyboard();
  initAuthOverlay();
  initErrorOverlays();
  initLegalOverlays();
  initPrefToggles();
  initAllNotifsModal();
  initFormModal();
  initAlertDialog();
  initAppliedFilters();
  initReviewedFilters();
  initPendingDateFilter();
  initPendingTypeFilter();
  initBulkReview();
  initBulkDrafts();
  initDraftsDateFilter();
  initHistoryModal();
  initHistoryListModal();
  initQuickPrompts();
  initTextareas();

  // Sync sidebar notification badge with PENDING data
  const notifBadge = $('#sidebar-notif-badge');
  if (notifBadge) notifBadge.textContent = PENDING.length;

  // Header AI toggle button
  initHeaderAIBtn();

  // Leave-assistant chat edit button
  $('#chat-leave-edit-btn')?.addEventListener('click', () => openFormModal('請假申請單'));

  // Welcome input: only the send button navigates to AI chat inner page
  $('#welcome-send-btn')?.addEventListener('click', () => {
    switchView('chat', { assistant: ASSISTANTS[0] });
  });

  // Mark all notifs as read → empty state
  $('#notif-mark-all-read')?.addEventListener('click', (e) => {
    e.stopPropagation();
    const list = $('#system-list');
    if (list) list.innerHTML = `
      <div class="empty-state notif-empty-state">
        <i data-lucide="bell-off" class="icon"></i>
        <div class="h4">你沒有未讀通知</div>
      </div>
    `;
    const dot = $('#notif-dot');
    if (dot) dot.style.display = 'none';
    iconsRefresh();
  });

  // Form list search
  $('#form-list-search-input')?.addEventListener('input', (e) => {
    formListSearchQuery = e.target.value || '';
    renderFormCategories();
    iconsRefresh();
  });

  // Tasks
  initTaskMonthNav();
  renderTasksByDate();
  renderTodos();
  initTaskModal();
  initTaskMobileTabs();
  applyTaskLayoutForViewport();
  window.addEventListener('resize', applyTaskLayoutForViewport);

  // Home clock widget
  initHomeClock();

  // Build attendance calendar + week punch table + dashboard chat
  renderAttendanceCalendar();
  initCalendarNav();
  renderWeekPunch();
  initWeekNav();
  renderDashChat();
  renderAssistantsChat();
  renderTaskChat();
  initAttendanceModal();
  initAttendanceMonthNav();
  initDeptMonthNav();
  initSimpleMonthNav('sales-month-prev', 'sales-month-next', 'sales-month-label');
  initSimpleMonthNav('finance-month-prev', 'finance-month-next', 'finance-month-label');
  renderClientTable();
  renderDeptTasksDashboard();
  initDeptMemberModal();
  initDeptMemberSort();

  // Apply daily welcome background image
  applyDailyWelcomeBackground();

  // Fetch weather + show today's date in header
  applyHeaderWeather();

  // Home scroll → fade header to white over first 100px
  initHomeHeaderScroll();

  // Small touches that give the UI a little warmth
  applyWelcomePersonality();
  printDevConsoleEgg();

  iconsRefresh();

  // Auto-focus the welcome textarea on home so user can start typing immediately
  const focusWelcome = () => {
    const t = $('#welcome-input-textarea');
    if (t && $('#view-home')?.classList.contains('active')) t.focus({ preventScroll: true });
  };
  focusWelcome();
  requestAnimationFrame(focusWelcome);
  setTimeout(focusWelcome, 50);
});

// ---------- Delight touches ----------
// Rotate the welcome greeting + placeholder prompts so opening the home
// page feels slightly different each time rather than static.
function applyWelcomePersonality() {
  const greetingEl = $('#welcome-greeting');
  const emojiEl = $('#welcome-emoji');
  const promptEl = $('#welcome-prompt');
  const input = $('#welcome-input-textarea');
  if (!greetingEl && !promptEl && !input) return;

  const hour = new Date().getHours();
  let period;
  if (hour >= 5 && hour < 11) period = 'morning';
  else if (hour >= 11 && hour < 14) period = 'noon';
  else if (hour >= 14 && hour < 18) period = 'afternoon';
  else if (hour >= 18 && hour < 23) period = 'evening';
  else period = 'night';

  const pool = {
    morning:   { greet: '早安', emoji: '☀️', prompts: ['今天想完成什麼？', '一杯咖啡準備好了嗎？', '先從哪件事開始？'] },
    noon:      { greet: '午安', emoji: '🍱', prompts: ['吃飽再衝下午！', '今天還剩什麼沒搞定？'] },
    afternoon: { greet: '下午好', emoji: '👋', prompts: ['下午再加把勁！', '還差什麼等你收尾？', '需要幫忙？'] },
    evening:   { greet: '晚安', emoji: '🌙', prompts: ['辛苦了，還有事沒結束嗎？', '做個總結再下班吧'] },
    night:     { greet: '夜深了', emoji: '🌙', prompts: ['早點休息，明天再戰', '還在加班嗎？'] }
  };
  const p = pool[period];

  if (greetingEl) greetingEl.textContent = p.greet;
  if (emojiEl) emojiEl.textContent = p.emoji;
  if (promptEl) promptEl.textContent = p.prompts[Math.floor(Math.random() * p.prompts.length)];

  if (input) {
    const prompts = [
      '問 AI,例如「幫我請特休」',
      '例如「本月工時統計」',
      '例如「推薦今天要做的事」',
      '例如「幫我產出週報草稿」',
      '例如「這季部門業績如何？」'
    ];
    input.placeholder = prompts[Math.floor(Math.random() * prompts.length)];
  }
}

// Easter egg for curious developers who open DevTools.
function printDevConsoleEgg() {
  try {
    console.log(
      '%cNexus OA',
      'color:#4f46e5;font-size:22px;font-weight:700;letter-spacing:-0.5px'
    );
    console.log(
      '%c看看這個介面的原始碼，你可能也是我們在找的人 — careers@ikala.com',
      'color:#64748b;font-size:12px'
    );
  } catch (_) { /* noop */ }
}
