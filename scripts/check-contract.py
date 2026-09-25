"""Validate the versioned OpenAPI document and its examples."""
from pathlib import Path
import sys

from openapi_spec_validator import validate_spec
from jsonschema import RefResolver
from openapi_schema_validator import OAS30Validator
import yaml


class UniqueKeysLoader(yaml.SafeLoader):
    pass


def unique_mapping(loader, node, deep=False):
    result = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=deep)
        if key in result:
            raise ValueError(f"Duplicate YAML key: {key}")
        result[key] = loader.construct_object(value_node, deep=deep)
    return result


UniqueKeysLoader.add_constructor(
    yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, unique_mapping)


def local_refs(value):
    if isinstance(value, dict):
        for key, child in value.items():
            if key == "$ref" and (not isinstance(child, str) or not child.startswith("#/")):
                raise ValueError("Contract references must stay inside this document")
            local_refs(child)
    elif isinstance(value, list):
        for child in value:
            local_refs(child)


def main():
    path = Path(sys.argv[1]) if len(sys.argv) == 2 else Path("docs/openapi.yaml")
    spec = yaml.load(path.read_text(), Loader=UniqueKeysLoader)
    if not isinstance(spec, dict) or spec.get("openapi") != "3.0.3":
        raise ValueError("Expected the agreed OpenAPI 3.0.3 contract")
    local_refs(spec)
    validate_spec(spec)
    resolver = RefResolver.from_schema(spec)
    def examples(value):
        if isinstance(value, dict):
            if "example" in value:
                schema = value.get("schema", value)
                if "type" in schema or "$ref" in schema:
                    OAS30Validator(schema, resolver=resolver, format_checker=OAS30Validator.FORMAT_CHECKER).validate(value["example"])
            for child in value.values():
                examples(child)
        elif isinstance(value, list):
            for child in value:
                examples(child)
    examples(spec)
    identifiers = set()
    methods = {"get", "post", "put", "patch", "delete", "options", "head", "trace"}
    for path_item in spec["paths"].values():
        for method, operation in path_item.items():
            if method not in methods:
                continue
            identifier = operation.get("operationId")
            if not identifier or identifier in identifiers:
                raise ValueError("Each operation needs a unique operationId")
            identifiers.add(identifier)
    print(f"OpenAPI valid: {len(identifiers)} operations, local references, unique YAML keys")


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(f"Contract validation failed: {error}", file=sys.stderr)
        sys.exit(1)
