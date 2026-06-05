-- HR Core domain tables (员工管理 feature). SCHEMA ONLY — faithfully normalized
-- from the PRD「【Feature】员工管理」six-tab employee record. Business logic
-- (CRUD / import / export / batch delete / state machine) is NOT implemented in
-- this milestone. hr_employees is the employee single-source-of-truth: all 1:1
-- section fields are typed columns; 内部经历/异动 (1:N) is a separate table.
-- Tenant-scoped with RLS like all iam_* tables.

CREATE TABLE hr_org_units (
    id                  text PRIMARY KEY,
    tenant_id           text NOT NULL REFERENCES iam_tenants(id),
    parent_id           text,
    code                text,
    name                text NOT NULL,
    manager_employee_id text,
    level               int NOT NULL DEFAULT 0,
    sort_order          int NOT NULL DEFAULT 0,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz
);
CREATE INDEX ix_hr_org_units_tenant_parent ON hr_org_units (tenant_id, parent_id);

CREATE TABLE hr_employees (
    id          text PRIMARY KEY,
    tenant_id   text NOT NULL REFERENCES iam_tenants(id),

    -- ── 分页1 基本资料 ───────────────────────────────────────────────
    employee_no       text,                       -- 员工编号 (auto, e.g. IKL030)
    card_no           text,                       -- 门禁/打卡卡号
    photo_url         text,
    nationality_type  text NOT NULL DEFAULT 'local' CHECK (nationality_type IN ('local','foreign')),
    name_zh           text,                       -- 中文姓名
    first_name        text,
    last_name         text,
    company_email     text,                       -- 公司 Email
    personal_email    text,
    office_phone_ext  text,                       -- 办公室电话(分机)
    gender            text,                       -- 男 / 女
    birthday          date,
    birthplace        text,
    nationality       text,                       -- 国籍 (foreign only)
    marital_status    text,                       -- 未婚 / 已婚
    has_criminal_record boolean,                  -- 是否有前科
    -- 法规身份 (local)
    id_number          text,                      -- 身分证号
    nhi_subsidy_identity text,                    -- 健保补助身份: 无/中低收入户/低收入户
    indigenous_identity  boolean,                 -- 原住民身份
    disability_category  text,                    -- 身心障碍类别: 无/视觉/听觉/肢体/其他
    disability_level     text,                    -- 身心障碍身份: 无/轻度/中度/重度/极重度
    -- 外籍员工资料 (foreign only)
    passport_no        text,
    passport_name      text,
    entry_date         date,                      -- 入境日期
    arc_no             text,                      -- 居留证号 (ARC)
    arc_expiry         date,                      -- 居留证到期日
    tax_id             text,                      -- 税籍编号
    work_permit_no     text,                      -- 工作证号
    work_permit_expiry date,                      -- 工作证到期日
    contract_expiry    date,                      -- 契约到期日
    agency             text,                      -- 仲介单位
    -- 生理资料
    blood_type   text,                            -- A/B/O/AB
    height_cm    numeric(5,2),
    weight_kg    numeric(5,2),

    -- ── 分页2 在职资料 ───────────────────────────────────────────────
    account_id        text REFERENCES iam_accounts(id),  -- link to login account (nullable)
    company_name      text,
    org_unit_id       text REFERENCES hr_org_units(id),  -- 部门
    title             text,                       -- 职称
    job_grade         text,                       -- 职等 P1..M3
    job_level         text,                       -- 职级 初/中/高/资深
    is_manager        boolean NOT NULL DEFAULT false,
    supervisor_employee_id text,                  -- 上级
    deputy_employee_id     text,                  -- 代理人
    hire_date         date,                       -- 到职日期
    probation_end_date date,                      -- 试用期满日
    expected_regularization_date date,            -- 预计转正日期
    recruitment_source text,                      -- 就职来源
    employment_status text NOT NULL DEFAULT 'pending'
        CHECK (employment_status IN ('probation','active','leave','resigned','pending')),
    shift             text,                       -- 班别: 一般/轮班/弹性
    category          text CHECK (category IS NULL OR category IN ('fulltime','contract','parttime','intern','other')), -- 身分类别
    seniority_start_date date,                    -- 年资起算日 (工作/特休年资为系统计算, 不落库)
    clock_in_out      boolean,                    -- 上下班刷卡
    responsibility_type text,                     -- 责任制/非责任制
    remark            text,
    -- status-conditional (PRD §5.5 C)
    leave_start_date  date,                       -- 留停开始日
    leave_end_date    date,                       -- 留停结束日
    resign_date       date,                       -- 离职日期
    resign_reason     text,                       -- 自请离职/资遣/退休/合约到期

    -- ── 分页3 学历兵役 ───────────────────────────────────────────────
    highest_education text,                       -- 高中职/专科/大学/硕士/博士
    degree            text,                       -- BA/BS/MBA/MS/PhD
    school_name       text,
    major             text,
    enrollment_date   date,
    graduation_status text,                       -- 毕业/肄业
    graduation_date   date,
    withdrawal_date   date,
    military_status   text,                       -- 已役/免役/未役/替代役
    military_branch   text,                       -- 陆/海/空/陆战队
    military_rank     text,

    -- ── 分页4 通讯资料 ───────────────────────────────────────────────
    mobile            text,                       -- 行动电话 (shown in list / search)
    household_phone   text,                       -- 户籍电话
    contact_phone     text,                       -- 通讯电话
    household_address text,                       -- 户籍地址
    contact_address   text,                       -- 通讯地址
    emergency_relationship text,                  -- 紧急联络人: 关系
    emergency_name    text,
    emergency_phone   text,
    emergency_address text,

    -- ── 分页5 保险资料 ───────────────────────────────────────────────
    labor_insurance_date     date,                -- 劳保投保日期
    labor_insurance_grade    text,                -- 劳保等级
    labor_insurance_salary   numeric(12,2),       -- 劳保薪资 (NTD)
    occupational_injury_grade text,               -- 职灾投保等级
    occupational_injury_salary numeric(12,2),     -- 职灾投保薪资 (NTD)
    nhi_date          date,                       -- 健保投保日期
    nhi_grade         text,                       -- 健保等级
    nhi_amount        numeric(12,2),              -- 健保投保金额 (NTD)
    nhi_dependents       int,                     -- 健保眷属人数
    disabled_dependents  int,                     -- 身障眷属人数
    subsidized_dependents int,                    -- 补助身份人数

    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);
CREATE UNIQUE INDEX uq_hr_employees_tenant_no ON hr_employees (tenant_id, employee_no) WHERE employee_no IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX ix_hr_employees_tenant_org ON hr_employees (tenant_id, org_unit_id);          -- 部门筛选
CREATE INDEX ix_hr_employees_tenant_status ON hr_employees (tenant_id, employment_status); -- 状态筛选
CREATE INDEX ix_hr_employees_tenant_category ON hr_employees (tenant_id, category);        -- 类别筛选

-- 内部经历 / 异动历史 (1:N). Latest row (end_date NULL / is_current) = 现职.
CREATE TABLE hr_employee_assignments (
    id            text PRIMARY KEY,
    tenant_id     text NOT NULL REFERENCES iam_tenants(id),
    employee_id   text NOT NULL REFERENCES hr_employees(id),
    start_date    date,                                  -- 任职起日
    end_date      date,                                  -- 任职迄日 (NULL = 现职)
    change_reason text,                                  -- 新进/转调/升迁/降调/留停复职
    org_unit_id   text REFERENCES hr_org_units(id),      -- 部门
    title         text,                                  -- 职务
    category      text,                                  -- 身分类别
    is_current    boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_hr_emp_assignments_employee ON hr_employee_assignments (tenant_id, employee_id);

-- RLS (same pattern as the iam_* tables).
DO $$
DECLARE
    t text;
    tables text[] := ARRAY['hr_org_units','hr_employees','hr_employee_assignments'];
BEGIN
    FOREACH t IN ARRAY tables LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$
            CREATE POLICY %1$s_tenant_isolation ON %1$I
            USING (tenant_id = current_setting('app.current_tenant', true))
            WITH CHECK (tenant_id = current_setting('app.current_tenant', true))
        $p$, t);
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO app_user', t);
    END LOOP;
END
$$;
