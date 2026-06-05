-- HR Core domain tables (员工管理 feature). SCHEMA ONLY — derived faithfully from
-- the PRD「【Feature】员工管理」. Business logic (CRUD / import / export / batch
-- delete / state machine) is NOT implemented in this milestone. Queryable/state
-- fields are real columns (for the list/filter/search/dashboard the PRD needs);
-- the self-contained optional tabs are JSONB sections (keys documented inline) and
-- can be normalized later. Tenant-scoped with RLS like all iam_* tables.

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

    -- identity / 基本资料 (queryable core)
    employee_no       text,                              -- 员工编号 (auto, e.g. IKL030)
    card_no           text,                              -- 门禁/打卡卡号
    photo_url         text,
    nationality_type  text NOT NULL DEFAULT 'local' CHECK (nationality_type IN ('local','foreign')),
    name_zh           text,                              -- 中文姓名
    first_name        text,
    last_name         text,
    company_email     text,                              -- 公司 Email
    personal_email    text,
    office_phone_ext  text,
    mobile            text,                              -- 行动电话 (shown in list / search)
    gender            text,                              -- 男 / 女
    birthday          date,
    birthplace        text,
    nationality       text,                              -- foreign only
    marital_status    text,                              -- 未婚 / 已婚
    has_criminal_record boolean,

    -- 在职资料 (queryable / state)
    account_id        text REFERENCES iam_accounts(id),  -- link to login account (nullable)
    company_name      text,
    org_unit_id       text REFERENCES hr_org_units(id),  -- 部门
    title             text,                              -- 职称
    job_grade         text,                              -- 职等 P1..M3
    job_level         text,                              -- 职级 初/中/高/资深
    is_manager        boolean NOT NULL DEFAULT false,    -- 管理职
    supervisor_employee_id text,                         -- 上级
    deputy_employee_id     text,                         -- 代理人
    hire_date         date,                              -- 到职日期
    probation_end_date date,
    expected_regularization_date date,
    recruitment_source text,                             -- 就职来源
    employment_status text NOT NULL DEFAULT 'pending'
        CHECK (employment_status IN ('probation','active','leave','resigned','pending')),
    shift             text,                              -- 班别: 一般/轮班/弹性
    category          text,                              -- 身分类别: fulltime/contract/parttime/intern/other
    seniority_start_date date,                           -- 年资起算日
    clock_in_out      boolean,                           -- 上下班刷卡
    responsibility_type text,                            -- 责任制/非责任制
    remark            text,

    -- status-conditional (PRD §5.5 C)
    leave_start_date  date,                              -- 留停开始日
    leave_end_date    date,                              -- 留停结束日
    resign_date       date,                              -- 离职日期
    resign_reason     text,                              -- 自请离职/资遣/退休/合约到期

    -- optional self-contained tabs as JSONB sections (keys documented):
    regulatory_identity jsonb NOT NULL DEFAULT '{}', -- 法规身份: id_number, nhi_subsidy_identity, indigenous, disability_category, disability_level
    foreign_profile     jsonb NOT NULL DEFAULT '{}', -- 外籍: passport_no/name, entry_date, arc_no, arc_expiry, tax_id, work_permit_no, work_permit_expiry, contract_expiry, agency
    physiological       jsonb NOT NULL DEFAULT '{}', -- 生理: blood_type, height_cm, weight_kg
    education           jsonb NOT NULL DEFAULT '{}', -- 学历: highest_education, degree, school_name, major, enrollment_date, graduation_status, graduation_date, withdrawal_date
    military_service    jsonb NOT NULL DEFAULT '{}', -- 兵役: status, branch, rank
    contact             jsonb NOT NULL DEFAULT '{}', -- 通讯: household_phone, contact_phone, household_address, contact_address
    emergency_contact   jsonb NOT NULL DEFAULT '{}', -- 紧急联络人: relationship, name, phone, address
    insurance           jsonb NOT NULL DEFAULT '{}', -- 保险: labor_*, occupational_*, nhi_*

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
