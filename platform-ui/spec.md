# Nexus OA 平台介面規格（spec.md）

> 基於 [`design-system.md`](../design-system.md) 的視覺規範，使用 shadcn/ui 元件庫、Indigo-600 為 Primary 色、Light purple（`hsl(243 100% 96%)`）為 Tertiary 作用中狀態色。

---

## 1. 全域布局（Global Layout）

整體採用「左側固定導覽列 + 右側主內容（Header + Content）」的經典 SaaS 布局。

### 1.1 左側導覽列（Sidebar）

固定於畫面最左側，寬度 240px（展開）/ 52px（收合）。參考 design-system `Sidebar` 元件。

| 位置 | 項目 | 圖示（Lucide） | 行為 |
|------|------|----------------|------|
| 頂部 | **Nexus logo** | — | 點擊回主頁；使用 [`logo.svg`](../logo.svg)（高度 20px） |
| 主選單 | 主頁 | `house` | 切換至主頁 |
| 主選單 | 表單申請 | `file-text` | 切換至表單申請頁 |
| 主選單 | AI 助手列表 | `sparkles` | 切換至 AI 助手列表 |
| 主選單 | 歷史紀錄 | `history` | 切換至歷史紀錄 |
| 底部 | **新對話** | `plus` | Primary 按鈕樣式，開啟新 AI 對話 |
| 底部 | 工作區設定 | `settings` | 開啟工作區設定 |

- **Active state**：Tertiary 底色（light purple）+ Primary 文字/Icon 色
- **Hover state**：`general/accent` 底色
- 分隔：底部項目與主選單以 `Separator` 分開

### 1.2 Header

固定於右側頂部，高度 56px，右對齊三個項目。

| 項目 | 行為 |
|------|------|
| 通知 `bell` 圖示 | 點擊顯示通知 popover，右上角紅點顯示未讀計數 |
| 設定 `settings` 圖示 | 進入設定頁 |
| 頭像 Avatar（32px 圓形） | 點擊下拉選單：個人資料、偏好設定、**登出** |

---

## 2. 主頁（Home）

最上方為 **Underline style Tabs**，兩個 Tab：**主頁** / **通知**。

### 2.1 「主頁」Tab

由上而下：

#### A. 歡迎詞 + AI 問答框
- `heading 2`（30px）：「早安，Tammy 👋 今天想完成什麼？」
- 副標（`paragraph` / `muted-foreground`）：「問我任何問題，或直接提交表單」
- **Chat Input**：大型輸入框 + `send` 送出按鈕，placeholder「輸入你的問題，例如：幫我申請下週一的特休」

#### B. 常用 AI 助手（橫向卡片）
- 標題：`heading 4`「常用 AI 助手」+ 右側「查看全部」Link Button
- **橫向滾動容器**，預設顯示 **3.5 個 Card**（暗示可往右滑），總共 **12 個助手**
- 右側箭頭 Icon Button（`chevron-right`）可捲動
- Card 內容：
  - 大 Emoji 圖示（48px）
  - Title（`paragraph medium`）
  - 2 行描述（`paragraph small` / `muted-foreground`）
- 12 個助手建議：
  1. 📝 請假助手 — 快速產生請假申請
  2. 💰 報銷助手 — 掃描發票自動填寫
  3. ✈️ 差旅助手 — 規劃行程與核銷
  4. 📊 報表分析師 — 分析銷售與 KPI
  5. 📧 郵件撰寫 — 產生正式商務信件
  6. 🗓️ 會議秘書 — 會議摘要與待辦
  7. 🔍 政策查詢 — 公司規章即時問答
  8. 👥 HR 小幫手 — 員工資訊查詢
  9. 📦 資產管理 — 報修與領用
  10. 🎯 OKR 教練 — 協助撰寫目標
  11. 📚 知識庫 — 內部文件搜尋
  12. 🛠️ IT 支援 — 排除常見技術問題

#### C. 常用申請表單（3 欄）
- 標題：`heading 4`「常用申請表單」
- 3 欄 Card，每欄標題 + 5 個可點擊表單列

| 財務表單 | 人力資源表單 | 個人申請表單 |
|----------|-------------|-------------|
| 費用報銷單 | 請假申請單 | 名片申請單 |
| 差旅費核銷單 | 加班申請單 | 識別證申請 |
| 付款申請單 | 出差申請單 | 物資領用單 |
| 請款單 | 職務代理申請 | 停車位申請 |
| 預算追加申請 | 在職證明申請 | 個人資料異動 |

- 每列項目為 `Item` 元件：左側圖示 + 中間標題 + 右側 `chevron-right`
- Hover 顯示 `accent` 底色

### 2.2 「通知」Tab

兩欄式布局（**Resizable Panels**，左 40% / 右 60%）。

#### 左欄：通知清單（垂直分兩段）

**A. 待審核列表**
- Section 標題：`heading 4`「待審核 (4)」+ Badge `Primary pill`
- 每列使用 `Card Item`：
  - 狀態 Badge：`待審核`（Outline yellow）
  - 表單標題：`paragraph medium`
  - 兩行描述（申請人 + 摘要）：`paragraph small / muted`
  - 右下角時間：`paragraph mini / muted`

範例項目：
1. 請假申請單 — 王小明 / 申請 4/20 特休一天
2. 費用報銷單 — 李芳華 / 客戶餐敘 NT$3,200
3. 加班申請單 — 陳大文 / 4/18 加班 3 小時
4. 出差申請單 — 林美玲 / 東京研討會 3 天

**B. 系統通知列表**
- Section 標題：「系統通知」
- 每列同上，但狀態 Badge 改為 `系統` / `已完成` / `提醒`

範例項目：
1. 你的 4/15 費用報銷已核准
2. 4 月薪資單已發布，請查收
3. 年度健檢預約截止日 4/30
4. 公司政策更新：差旅新規定生效

#### 右欄：AI 審核助理對話

- 上方 AI 對話區（可捲動）
- AI 開場訊息：
  > 👋 你目前有 4 個表單待審核，是否要我依序協助你判斷？
- 選中某筆待審時，AI 顯示：
  - 申請緣由摘要
  - 風險評估（預算/人力/合規）
  - 建議：✅ 建議核准 / ⚠️ 建議退回補件 / ❌ 建議不通過
- 訊息下方三個操作按鈕（`Button Group`）：
  - `Destructive`「不通過」
  - `Outline`「退回補件」
  - `Primary`「核准」

- 下方 **Chat Input**（`Chat Input` 元件）
- Chat Input **上方** 3 個 **快捷提示按鈕**（`Button / Ghost Muted / Small`）：
  1. 📅 顯示該成員本月假單
  2. ⏱️ 該成員工作時數統計
  3. 📊 本月人員請假統計

---

## 3. 表單申請（Form Request）

兩欄式布局（**Resizable**，左 45% / 右 55%）。

### 3.1 左欄：分類表單清單

使用 `Accordion`，每個分類預設展開。

#### 人事考勤類
- 請假申請單 🗓️
- 加班申請單 ⏰
- 出差申請單 ✈️

#### 財務費用類
- 費用報銷單 💳
- 差旅費核銷單 🧳
- 付款申請單 💰

#### 行政庶務類
- 物資領用單 📦
- 用印申請單 🔖
- 資產報修申請單 🔧

#### 通用管理類
- 通用簽呈 📝

每個項目為 `Item` 元件：Emoji + 標題 + 描述（`paragraph mini`）+ 右側「填寫」`Button / Primary / Small`。

上方有 **搜尋 Input**（`Input` with `search` prefix icon）與 **熱門標籤**（`Badge Ghost`）。

### 3.2 右欄：AI 表單對話問答

- `heading 4`「AI 表單助理」
- 副標：「不確定該用哪張表單？問我！」
- 對話區：AI 以自然語言引導
  > 你想處理什麼事情？我可以推薦最適合的表單並協助你填寫。
- 使用者範例輸入：「我下週要去東京出差 3 天」
- AI 回覆卡片：推薦「出差申請單」+ 自動帶出欄位草稿
- 底部 **Chat Input** + 快捷按鈕：
  - 📝 我要請假
  - 💸 我要報銷
  - 🖨️ 我要用印
  - ❓ 不確定用哪張

---

## 4. AI 助手列表（AI Assistants）

### 4.1 列表視圖

- 頁首：`heading 2`「AI 助手」+ 搜尋框 + 分類篩選（`Tabs`：全部 / 工作流 / 文件 / 分析 / IT）
- **3 × 3 = 9 張 Card 網格**（一行 3 個）
- 每張 Card：
  - 左上 Emoji 圖示（48px，圓角背景 Tertiary）
  - 標題（`heading 4`）
  - 描述（`paragraph small`，2-3 行）
  - 底部：「開始對話」`Button / Primary / Small` + 右下使用次數 Badge
- Hover：`shadow-md` + 微上浮

9 張 Card 建議：
1. 📝 請假助手
2. 💰 報銷助手
3. ✈️ 差旅助手
4. 📊 報表分析師
5. 📧 郵件撰寫
6. 🗓️ 會議秘書
7. 🔍 政策查詢
8. 👥 HR 小幫手
9. 🛠️ IT 支援

### 4.2 進入對話後的布局

點擊 Card 後**切換成兩欄布局**（類似 ChatGPT）：

#### 左欄（240px）：
- **返回主頁** Link Button（`chevron-left` + 「返回主頁」）
- **新對話** Primary 按鈕
- 分隔
- **歷史對話清單**（`Item` 列表）：顯示過往與此助手的對話標題 + 時間

#### 右欄：AI 對話視圖
- 頂部顯示當前助手資訊（Avatar emoji + 名稱 + 描述）
- 中間對話區：使用者訊息（靠右，tertiary 底色）+ AI 訊息（靠左，muted 底色，含頭像）
- 底部 **Chat Input**（含附件、錄音、送出）

---

## 5. 歷史紀錄（History）

頂部 **Underline Tabs**：
- **問答歷史紀錄**
- **表單申請紀錄**

### 5.1 問答歷史紀錄 Tab

- `Data Table` 呈現：
  - 欄位：對話標題 / 所屬助手 / 最後更新時間 / 訊息數 / 操作
  - 操作：`Icon Button / Ghost` — 繼續對話、分享、刪除
- 左側篩選：日期範圍（`Date Range Picker`）、助手篩選（`Select`）
- 頂部有搜尋 `Input`

### 5.2 表單申請紀錄 Tab

- `Data Table` 呈現：
  - 欄位：表單編號 / 表單標題 / 類型 / 狀態（Badge）/ 申請日期 / 核准狀態 / 操作
  - 狀態 Badge 色：
    - `待審核` — Outline
    - `審核中` — Primary
    - `已核准` — Ghost green
    - `已退回` — Destructive
    - `已撤回` — Secondary
- 支援分頁（`Pagination`）
- 支援匯出（`Button / Outline`：匯出 CSV）

---

## 6. 設計 Tokens 對應

| 用途 | Token |
|------|-------|
| Primary 按鈕 / Active Icon | `--primary` (Indigo-600 `#4f46e5`) |
| Tertiary 作用中底色 | `--tertiary` (`hsl(243 100% 96%)`) |
| 卡片陰影 | shadow `md` |
| 下拉選單陰影 | shadow `sm` |
| 彈窗陰影 | shadow `lg` |
| 主要字型 | Inter |
| 標題 2 | 30px / 600 |
| 內容 | 16px / 400 |
| 說明文字 | 14px / 400 / muted-foreground |
| 圓角 | `rounded-md`（預設）/ `rounded-sm`（小元件）|
| 間距 | 以 8px 為基礎的 t-shirt 刻度（xs/md/lg/xl/2xl/3xl） |

---

## 7. 互動與狀態

- 所有按鈕支援 Focus ring（Primary 2px outline）
- 所有 hover 有 150ms ease-out 過渡
- Loading 狀態：使用 `Spinner` 或 `Skeleton`
- 空狀態：使用 `Empty` 元件

---

## 8. 檔案結構

```
platform-ui/
├── spec.md           # 本規格文件
├── index.html        # 主 HTML，包含所有頁面與路由
├── styles.css        # 設計系統 tokens + 元件樣式
└── app.js            # 頁面切換、互動邏輯、示例資料
```
