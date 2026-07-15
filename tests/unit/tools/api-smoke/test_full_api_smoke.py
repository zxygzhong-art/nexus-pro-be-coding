import importlib.util
import pathlib
import sys
import unittest


MODULE_PATH = pathlib.Path(__file__).resolve().parents[4] / "tools" / "api-smoke" / "full_api_smoke.py"
SPEC = importlib.util.spec_from_file_location("full_api_smoke", MODULE_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"cannot load {MODULE_PATH}")
SMOKE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = SMOKE
SPEC.loader.exec_module(SMOKE)


class SmokeCoveragePlanTests(unittest.TestCase):
    # test_plan_covers_openapi_once_by_category keeps generated checks limited
    # to routes that do not already have a behavioral case.
    def test_plan_covers_openapi_once_by_category(self) -> None:
        plan = SMOKE.build_case_plan()

        self.assertTrue(plan.openapi_routes)
        self.assertIn("PUT /v1/me/password", plan.openapi_routes)
        self.assertFalse(plan.behavioral_routes & plan.auth_boundary_routes)
        self.assertEqual(
            plan.openapi_routes,
            plan.behavioral_routes | plan.auth_boundary_routes,
        )
        self.assertEqual(
            len(plan.auth_boundary_cases),
            len(plan.openapi_routes - plan.behavioral_routes),
        )

    # test_generated_cases_are_side_effect_free ensures automatic coverage can
    # never carry credentials into a documented mutation handler.
    def test_generated_cases_are_side_effect_free(self) -> None:
        plan = SMOKE.build_case_plan()

        for case in plan.auth_boundary_cases:
            self.assertIsNone(case.auth, case.name)
            self.assertEqual(401, case.expected, case.name)
            self.assertIsNone(case.json_body, case.name)
            self.assertIsNone(case.raw_body, case.name)
            self.assertIsInstance(case.path, str, case.name)
            self.assertNotIn("{", case.path, case.name)
            self.assertNotIn("}", case.path, case.name)
            method, documented_path = case.route_key.split(" ", 1)
            self.assertEqual(method, case.method, case.name)
            self.assertEqual(
                SMOKE.materialize_openapi_path(documented_path),
                case.path,
                case.name,
            )

    # test_behavioral_routes_remain_documented catches stale manual cases when
    # the OpenAPI contract is renamed or removed.
    def test_behavioral_routes_remain_documented(self) -> None:
        plan = SMOKE.build_case_plan()

        self.assertLessEqual(plan.behavioral_routes, plan.openapi_routes)

    # test_openapi_inventory_stays_inside_paths prevents component names such
    # as options from being counted as HTTP operations.
    def test_openapi_inventory_stays_inside_paths(self) -> None:
        raw = """openapi: 3.0.3
paths:
  /v1/example:
    put:
      responses: {}
components:
  schemas:
    options:
      type: object
"""

        self.assertEqual(
            {"PUT /v1/example"},
            SMOKE.openapi_route_keys_from_text(raw),
        )


if __name__ == "__main__":
    unittest.main()
