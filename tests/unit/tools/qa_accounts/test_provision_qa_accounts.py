import importlib.util
import os
from pathlib import Path
import unittest
from unittest import mock


REPO_ROOT = Path(__file__).resolve().parents[4]
PROVISIONER = REPO_ROOT / "tools" / "qa-accounts" / "provision_qa_accounts.py"


def load_provisioner():
    spec = importlib.util.spec_from_file_location("qa_account_provisioner_under_test", PROVISIONER)
    if spec is None or spec.loader is None:
        raise RuntimeError("unable to load QA account provisioner")
    module = importlib.util.module_from_spec(spec)
    with mock.patch.dict(os.environ, {"QA_TENANT_ID": "qa15-hr"}, clear=False):
        spec.loader.exec_module(module)
    return module


class QAAccountProvisionerTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.provisioner = load_provisioner()

    def test_hr_role_projects_organization_and_turnover_without_new_api_privileges(self):
        permissions = self.provisioner.PERMISSION_SETS["ps-qa-hr-admin"]["permissions"]
        grants_by_menu = {permission.get("menu_key"): permission for permission in permissions}

        for menu_key in ("hr.organization", "hr.turnover"):
            with self.subTest(menu_key=menu_key):
                self.assertEqual(
                    (grants_by_menu[menu_key]["resource"], grants_by_menu[menu_key]["action"], grants_by_menu[menu_key]["scope"]),
                    ("hr.employee", "read", "all"),
                )

        api_grants = {
            (permission["resource"], permission["action"], permission["scope"])
            for permission in permissions
        }
        self.assertIn(("hr.employee", "read", "all"), api_grants)
        self.assertNotIn(("hr.position", "read", "all"), api_grants)

    def test_non_default_tenant_keeps_the_existing_hr_permission_set_id(self):
        self.assertEqual(
            self.provisioner.permission_set_id("ps-qa-hr-admin"),
            "ps-qa-hr-admin-qa15-hr",
        )

    def test_attendance_manager_projects_every_documented_workspace_page(self):
        permissions = self.provisioner.PERMISSION_SETS["ps-qa-attendance-manager"]["permissions"]

        expected = {
            "attendance.overview": ("attendance.clock", "read", "all"),
            "attendance.clock": ("attendance.clock", "read", "all"),
            "attendance.leave_policy": ("attendance.leave", "read", "all"),
        }
        for menu_key, grant in expected.items():
            with self.subTest(menu_key=menu_key):
                self.assertIn(
                    (*grant, menu_key),
                    {
                        (
                            permission["resource"],
                            permission["action"],
                            permission["scope"],
                            permission.get("menu_key"),
                        )
                        for permission in permissions
                    },
                )


if __name__ == "__main__":
    unittest.main()
