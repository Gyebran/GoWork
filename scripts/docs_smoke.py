"""M11 contract checks against the disposable Docker API; never target production."""
import json
from pathlib import Path
import urllib.error
import urllib.request

from jsonschema import RefResolver
from openapi_schema_validator import OAS30ReadValidator, OAS30WriteValidator
from playwright.sync_api import sync_playwright, expect
import yaml


def verify(base):
    with urllib.request.urlopen(base + "/openapi.yaml", timeout=10) as response:
        raw = response.read()
    assert raw == Path("docs/openapi.yaml").read_bytes(), "Served contract differs from source"
    spec = yaml.safe_load(raw)
    resolver = RefResolver.from_schema(spec)
    covered = set()

    def resolve(value):
        if "$ref" in value:
            return resolver.resolve(value["$ref"])[1]
        return value

    def call(method, template, *, id=None, token=None, data=None, status=200, query="", raw_body=None, content_type="application/json", error_code=None):
        path = template.replace("{id}", id or "")
        op = spec["paths"][template][method.lower()]
        if data is not None and status < 400:
            schema = op["requestBody"]["content"]["application/json"]["schema"]
            OAS30WriteValidator(schema, resolver=resolver, format_checker=OAS30WriteValidator.FORMAT_CHECKER).validate(data)
        headers = {"Content-Type": content_type, "X-Request-ID": "m11-contract"}
        if token:
            headers["Authorization"] = "Bearer " + token
        body = raw_body if raw_body is not None else (json.dumps(data).encode() if data is not None else None)
        req = urllib.request.Request(base + path + query, data=body, method=method, headers=headers)
        try:
            response = urllib.request.urlopen(req, timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            payload = response.read()
            assert response.status == status, (method, template, response.status, status)
            assert response.headers["X-Request-ID"] == "m11-contract"
            definition = resolve(op["responses"][str(status)])
            for name, header in definition.get("headers", {}).items():
                # WWW-Authenticate is conditional for login's INVALID_CREDENTIALS.
                if name == "WWW-Authenticate" and template.endswith("/login"):
                    continue
                assert name in response.headers, (template, name)
                OAS30ReadValidator(header["schema"]).validate(response.headers[name])
            if status == 204:
                assert not payload
                result = None
            else:
                media = response.headers.get_content_type()
                schema = definition["content"][media]["schema"]
                result = json.loads(payload) if media == "application/json" else payload.decode()
                OAS30ReadValidator(schema, resolver=resolver, format_checker=OAS30ReadValidator.FORMAT_CHECKER).validate(result)
                if status >= 400:
                    assert result["request_id"] == response.headers["X-Request-ID"]
                    if error_code:
                        assert result["error"]["code"] == error_code
        if status < 400:
            covered.add((template, method.lower()))
        return result

    for path in ["/health", "/ready", "/docs", "/openapi.yaml"]:
        call("GET", path)
    login = call("POST", "/api/v1/auth/login", data={"email": "admin@gowork.dev", "password": "GoWork-demo-only-2026!"})
    token = login["data"]["access_token"]
    call("GET", "/api/v1/auth/me", token=token)
    user = call("POST", "/api/v1/users", token=token, status=201, data={
        "name": "Documentation Technician", "email": "m11@example.com", "password": "M11-example-only-password!", "role": "TECHNICIAN"})["data"]
    call("GET", "/api/v1/users", token=token, query="?role=TECHNICIAN&limit=2")
    call("GET", "/api/v1/users/{id}", id=user["id"], token=token)
    call("PATCH", "/api/v1/users/{id}", id=user["id"], token=token, data={"name": "Updated Technician"})
    asset_data = {"asset_code": "M11-DOCS", "name": "Documentation pump", "category": "Utilities", "location": "CI"}
    asset = call("POST", "/api/v1/assets", token=token, status=201, data=asset_data)["data"]
    call("GET", "/api/v1/assets", token=token, query="?search=Documentation&limit=2")
    call("GET", "/api/v1/assets/{id}", id=asset["id"], token=token)
    call("PATCH", "/api/v1/assets/{id}", id=asset["id"], token=token, data={"location": "CI updated"})
    order = call("POST", "/api/v1/work-orders", token=token, status=201, data={"asset_id": asset["id"], "title": "Inspect documentation pump"})["data"]
    assert order["assigned_to"] is None and order["completed_at"] is None
    call("GET", "/api/v1/work-orders", token=token, query="?status=OPEN")
    call("GET", "/api/v1/work-orders/{id}", id=order["id"], token=token)
    call("PATCH", "/api/v1/work-orders/{id}", id=order["id"], token=token, data={"priority": "HIGH", "description": ""})
    call("PATCH", "/api/v1/work-orders/{id}/assign", id=order["id"], token=token, data={"assigned_to": user["id"]})
    for target in ["IN_PROGRESS", "COMPLETED"]:
        completed = call("PATCH", "/api/v1/work-orders/{id}/status", id=order["id"], token=token, data={"status": target})["data"]
    assert completed["completed_at"] is not None
    logs = call("GET", "/api/v1/audit-logs", token=token, query="?entity_type=work_order&entity_id=" + order["id"])
    assert logs["meta"]["total"] == 6
    spare = call("POST", "/api/v1/assets", token=token, status=201, data=dict(asset_data, asset_code="M11-DELETE"))["data"]
    call("DELETE", "/api/v1/assets/{id}", id=spare["id"], token=token, status=204)
    # Representative documented errors, including the previously missing detail 422.
    call("GET", "/api/v1/users", status=401, error_code="UNAUTHENTICATED")
    tech = call("POST", "/api/v1/auth/login", data={"email": "m11@example.com", "password": "M11-example-only-password!"})["data"]["access_token"]
    call("GET", "/api/v1/users", token=tech, status=403, error_code="FORBIDDEN")
    call("GET", "/api/v1/assets/{id}", id=spare["id"], token=token, status=404, error_code="ASSET_NOT_FOUND")
    call("POST", "/api/v1/assets", token=token, data=asset_data, status=409, error_code="ASSET_CODE_CONFLICT")
    call("GET", "/api/v1/work-orders/{id}", id="bad-uuid", token=token, status=422, error_code="VALIDATION_ERROR")
    call("POST", "/api/v1/assets", token=token, raw_body=b'{', status=400, error_code="INVALID_JSON")
    call("POST", "/api/v1/assets", token=token, raw_body=b'{}', content_type="text/plain", status=415, error_code="UNSUPPORTED_MEDIA_TYPE")
    call("POST", "/api/v1/assets", token=token, raw_body=b' ' * (1024 * 1024 + 1), status=413, error_code="PAYLOAD_TOO_LARGE")
    expected = {(path, method) for path, item in spec["paths"].items() for method in item if method in {"get", "post", "patch", "delete", "put", "head", "options", "trace"}}
    assert covered == expected, expected - covered

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        page = browser.new_page()
        errors = []
        page.on("pageerror", lambda error: errors.append(str(error)))
        page.goto(base + "/docs", wait_until="networkidle")
        expect(page.locator(".opblock")).to_have_count(len(expected), timeout=30000)
        health = page.locator("#operations-Operations-health")
        health.locator(".opblock-summary").click()
        health.get_by_role("button", name="Try it out").click()
        health.get_by_role("button", name="Execute", exact=True).click()
        expect(health.locator(".live-responses-table .response-col_status")).to_contain_text("200")
        page.locator(".auth-wrapper").get_by_role("button", name="Authorize").click()
        page.locator(".dialog-ux input").fill(token)
        page.locator(".dialog-ux").get_by_role("button", name="Authorize", exact=True).click()
        page.locator(".dialog-ux").get_by_role("button", name="Close", exact=True).click()
        me = page.locator("#operations-Auth-getCurrentUser")
        me.locator(".opblock-summary").click()
        me.get_by_role("button", name="Try it out").click()
        me.get_by_role("button", name="Execute", exact=True).click()
        expect(me.locator(".live-responses-table .response-col_status")).to_contain_text("200")
        page.reload(wait_until="networkidle")
        assert page.evaluate("window.ui.authSelectors.authorized().size") == 0
        assert not errors, errors
        browser.close()
    print(f"Documentation smoke passed: {len(covered)} operations, schema/header checks, errors, Chromium render, Try it out and bearer authorization.", flush=True)
