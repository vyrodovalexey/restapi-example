#!/bin/sh
# =============================================================================
# init-keycloak.sh – Verify Keycloak realm, client and users are configured
#
# Keycloak imports the realm JSON on startup (--import-realm). This script
# waits for Keycloak to be ready and then verifies the import succeeded.
# If the realm was not auto-imported it creates the resources via the Admin
# REST API as a fallback.
#
# The script is idempotent: it can be run multiple times safely.
#
# NOTE: Uses wget (not curl) for compatibility with Alpine-based images
# that do not ship curl (e.g. hashicorp/vault).
# =============================================================================
set -e

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
KEYCLOAK_URL="${KEYCLOAK_URL:-http://keycloak:8090}"
ADMIN_USER="${KEYCLOAK_ADMIN:-admin}"
ADMIN_PASS="${KEYCLOAK_ADMIN_PASSWORD:-admin}"
REALM="${REALM_NAME:-restapi-test}"
CLIENT_ID="${CLIENT_ID:-restapi-server}"
CLIENT_SECRET="${CLIENT_SECRET:-restapi-server-secret}"

# Global auth token (set after login)
AUTH_TOKEN=""

# ---------------------------------------------------------------------------
# Helper: HTTP GET returning body to stdout
# ---------------------------------------------------------------------------
http_get() {
  if [ -n "${AUTH_TOKEN}" ]; then
    wget -qO- --header="Authorization: Bearer ${AUTH_TOKEN}" "$1" 2>/dev/null || true
  else
    wget -qO- "$1" 2>/dev/null || true
  fi
}

# ---------------------------------------------------------------------------
# Helper: HTTP POST with form data, returning body to stdout
# ---------------------------------------------------------------------------
http_post_form() {
  if [ -n "${AUTH_TOKEN}" ]; then
    wget -qO- \
      --header="Content-Type: application/x-www-form-urlencoded" \
      --header="Authorization: Bearer ${AUTH_TOKEN}" \
      --post-data="$2" "$1" 2>/dev/null || true
  else
    wget -qO- \
      --header="Content-Type: application/x-www-form-urlencoded" \
      --post-data="$2" "$1" 2>/dev/null || true
  fi
}

# ---------------------------------------------------------------------------
# Helper: HTTP POST with JSON body, returning body to stdout
# ---------------------------------------------------------------------------
http_post_json() {
  wget -qO- \
    --header="Content-Type: application/json" \
    --header="Authorization: Bearer ${AUTH_TOKEN}" \
    --post-data="$2" "$1" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Helper: HTTP GET returning status code
# ---------------------------------------------------------------------------
http_status() {
  wget -q -O /dev/null -S --header="Authorization: Bearer ${AUTH_TOKEN}" "$1" 2>&1 \
    | awk '/^  HTTP\//{print $2}' | tail -1
}

# ---------------------------------------------------------------------------
# Helper: wait for Keycloak to be ready
# ---------------------------------------------------------------------------
wait_for_keycloak() {
  echo "==> Waiting for Keycloak at ${KEYCLOAK_URL} ..."
  retries=0
  max_retries=60
  while [ "$retries" -lt "$max_retries" ]; do
    if wget -qO /dev/null "${KEYCLOAK_URL}/realms/master/.well-known/openid-configuration" 2>/dev/null; then
      echo "==> Keycloak is ready"
      return 0
    fi
    retries=$((retries + 1))
    echo "    attempt ${retries}/${max_retries} – retrying in 3s ..."
    sleep 3
  done
  echo "ERROR: Keycloak did not become ready in time"
  exit 1
}

# ---------------------------------------------------------------------------
# Helper: obtain admin access token
# ---------------------------------------------------------------------------
get_admin_token() {
  AUTH_TOKEN=""
  response=$(http_post_form \
    "${KEYCLOAK_URL}/realms/master/protocol/openid-connect/token" \
    "username=${ADMIN_USER}&password=${ADMIN_PASS}&grant_type=password&client_id=admin-cli")

  AUTH_TOKEN=$(echo "${response}" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')

  if [ -z "${AUTH_TOKEN}" ]; then
    echo "ERROR: Failed to obtain admin token"
    echo "Response: ${response}"
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# Check if realm exists
# ---------------------------------------------------------------------------
realm_exists() {
  code=$(http_status "${KEYCLOAK_URL}/admin/realms/${REALM}")
  [ "${code}" = "200" ]
}

# ---------------------------------------------------------------------------
# Create realm via Admin API (fallback if import did not run)
# ---------------------------------------------------------------------------
create_realm() {
  echo "==> Creating realm '${REALM}' via Admin API ..."
  http_post_json \
    "${KEYCLOAK_URL}/admin/realms" \
    "{\"realm\": \"${REALM}\", \"enabled\": true, \"displayName\": \"REST API Test Realm\", \"sslRequired\": \"none\"}" \
    > /dev/null
  echo "    Realm '${REALM}' created"
}

# ---------------------------------------------------------------------------
# Ensure client exists
# ---------------------------------------------------------------------------
ensure_client() {
  echo "==> Checking client '${CLIENT_ID}' in realm '${REALM}' ..."

  clients=$(http_get "${KEYCLOAK_URL}/admin/realms/${REALM}/clients?clientId=${CLIENT_ID}")

  if echo "${clients}" | grep -q "\"clientId\":\"${CLIENT_ID}\""; then
    echo "    Client '${CLIENT_ID}' already exists"
    return 0
  fi

  echo "    Creating client '${CLIENT_ID}' ..."
  http_post_json \
    "${KEYCLOAK_URL}/admin/realms/${REALM}/clients" \
    "{\"clientId\": \"${CLIENT_ID}\", \"name\": \"REST API Server\", \"enabled\": true, \"clientAuthenticatorType\": \"client-secret\", \"secret\": \"${CLIENT_SECRET}\", \"protocol\": \"openid-connect\", \"publicClient\": false, \"serviceAccountsEnabled\": true, \"standardFlowEnabled\": true, \"directAccessGrantsEnabled\": true, \"redirectUris\": [\"*\"], \"webOrigins\": [\"*\"]}" \
    > /dev/null
  echo "    Client '${CLIENT_ID}' created"
}

# ---------------------------------------------------------------------------
# Ensure realm roles exist
# ---------------------------------------------------------------------------
ensure_roles() {
  echo "==> Ensuring realm roles exist ..."

  for role in "api:read" "api:write" "admin"; do
    code=$(http_status "${KEYCLOAK_URL}/admin/realms/${REALM}/roles/${role}")

    if [ "${code}" = "200" ]; then
      echo "    Role '${role}' already exists"
    else
      http_post_json \
        "${KEYCLOAK_URL}/admin/realms/${REALM}/roles" \
        "{\"name\": \"${role}\"}" \
        > /dev/null
      echo "    Role '${role}' created"
    fi
  done
}

# ---------------------------------------------------------------------------
# Ensure user exists and has roles
# ---------------------------------------------------------------------------
ensure_user() {
  username="$1"
  password="$2"
  email="$3"
  shift 3
  roles="$*"

  echo "==> Ensuring user '${username}' exists ..."

  users=$(http_get "${KEYCLOAK_URL}/admin/realms/${REALM}/users?username=${username}&exact=true")

  user_id=""
  if echo "${users}" | grep -q "\"username\":\"${username}\""; then
    user_id=$(echo "${users}" | sed -n 's/.*"id":"\([^"]*\)".*"username":"'"${username}"'".*/\1/p')
    echo "    User '${username}' already exists (id=${user_id})"
  else
    http_post_json \
      "${KEYCLOAK_URL}/admin/realms/${REALM}/users" \
      "{\"username\": \"${username}\", \"enabled\": true, \"email\": \"${email}\", \"emailVerified\": true, \"credentials\": [{\"type\": \"password\", \"value\": \"${password}\", \"temporary\": false}]}" \
      > /dev/null

    # Fetch the newly created user ID
    users=$(http_get "${KEYCLOAK_URL}/admin/realms/${REALM}/users?username=${username}&exact=true")
    user_id=$(echo "${users}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
    echo "    User '${username}' created (id=${user_id})"
  fi

  # Assign roles
  if [ -n "${user_id}" ] && [ -n "${roles}" ]; then
    for role in ${roles}; do
      role_json=$(http_get "${KEYCLOAK_URL}/admin/realms/${REALM}/roles/${role}")

      if [ -n "${role_json}" ]; then
        role_id=$(echo "${role_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
        role_name=$(echo "${role_json}" | sed -n 's/.*"name":"\([^"]*\)".*/\1/p')

        http_post_json \
          "${KEYCLOAK_URL}/admin/realms/${REALM}/users/${user_id}/role-mappings/realm" \
          "[{\"id\": \"${role_id}\", \"name\": \"${role_name}\"}]" \
          > /dev/null
        echo "    Assigned role '${role}' to '${username}'"
      fi
    done
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  echo "============================================="
  echo " Keycloak Initialisation"
  echo "============================================="

  wait_for_keycloak

  echo "==> Obtaining admin token ..."
  get_admin_token
  echo "    Admin token obtained"

  # Check if realm was auto-imported
  if realm_exists; then
    echo "==> Realm '${REALM}' already exists (auto-imported)"
  else
    create_realm
  fi

  # Ensure all resources exist (idempotent)
  ensure_roles
  ensure_client
  ensure_user "test-user" "test-password" "test-user@example.com" "api:read"
  ensure_user "admin-user" "admin-password" "admin-user@example.com" "api:read" "api:write" "admin"

  echo ""
  echo "============================================="
  echo " Keycloak initialisation complete"
  echo "============================================="
  echo ""
  echo " Realm:         ${REALM}"
  echo " Client ID:     ${CLIENT_ID}"
  echo " Client Secret: ${CLIENT_SECRET}"
  echo " OIDC Issuer:   ${KEYCLOAK_URL}/realms/${REALM}"
  echo " Users:         test-user (api:read), admin-user (api:read, api:write, admin)"
}

main "$@"
