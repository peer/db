<!--
The permissions tab of the document edit page. It lists the users the document grants access to (with
a checkbox per permission action and a button to remove their access) and the access requests waiting
for a decision (which can be approved or denied; requests are recorded by the document request page).

It edits the edit session like the other tabs do: every decision is appended to the session as claim
changes right away, so the tab shows the session's document, the other tabs show what was decided
here, and other clients editing the same session see it as well. Save then commits it all together,
and Discard drops it, like for any other edit. It also means the permissions of the document being
edited can be changed from within the session which changes them.

Permissions are stored on the document itself: a HAS_PERMISSION reference claim points to a
PERMISSION_ACTIONS vocabulary value and carries a PERMISSION_USER sub-claim with the user and a
PERMISSION_SCOPE sub-claim with the self scope. An access request is the same shape under
HAS_REQUESTED_PERMISSION, plus an optional DESCRIPTION sub-claim with the note the requester wrote,
and approving it replaces the request claim with grants (which drops the note: it is about the
request, not about the access).

Every listed user gets a checkbox per action: checked for the actions the document grants them, which
unchecking removes (together with the actions which build on it), and unchecked for the actions they
hold through neither the document nor their roles, which checking grants (together with the actions it
requires and they lack). An action their roles already cover is not offered at all, because granting it
on the document would add nothing to what they can do.

Access is granted to a user who has not asked for it by naming them in the entry under the users with
access: it holds the same permissions those users have, so naming somebody shows what they could be
granted, and granting them any of it makes them one of those users, listed above. What they hold
already cannot be taken away there, which is what their own entry is for.

The roles of a user come from the user API (see UserGetAPI in user.go), which every user shown here is
looked up through, so the checkboxes and the decisions of a user who cannot be looked up are not
offered: without knowing what they already have, what to grant them is not known either.

Approving a request grants the requested action and, with it, the actions it requires which the user is
missing. Pending requests for any of the granted actions are approved along, so the same access is
never decided twice. Denying a request only removes it. It takes nothing away: what the user holds
through role grants stays theirs, and so does what the document already grants them.

Granting is offered only for the actions the caller holds themselves, because the server refuses a
change granting an action the caller does not hold (see checkChangePermission in permissions.go): such
an action can be neither checked nor approved here. Taking access away is not granting, so it stays
possible.
-->

<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClaimTypes } from "@/document"
import type { PermissionGrant, PermissionRequest } from "@/permissions"
import type { Identity, ValidationError } from "@/types"

import { computed, inject, ref } from "vue"
import { useI18n } from "vue-i18n"

import { hasDocumentPermission, hasUserDocumentPermission, hasUserRoleDocumentPermission, SCOPE_SELF } from "@/auth"
import Button from "@/components/Button.vue"
import CheckBox from "@/components/CheckBox.vue"
import WithDocument from "@/components/WithDocument.vue"
import { HAS_PERMISSION, PERMISSION_SCOPE, PERMISSION_USER } from "@/core"
import { HighConfidence } from "@/document"
import { saveChangeKey } from "@/fields"
import IdentityLabel from "@/partials/IdentityLabel.vue"
import InputIdentity from "@/partials/input/InputIdentity.vue"
import { useBusy } from "@/progress"
import {
  permissionActionClosure,
  permissionActionHint,
  permissionActionLabel,
  permissionActionOrder,
  permissionActions,
  permissionActionsWithout,
  permissionGrants,
  permissionRequests,
} from "@/permissions"
import { pickErrorMessage } from "@/validation"

const props = defineProps<{
  claims: DeepReadonly<ClaimTypes>
}>()

const { t } = useI18n({ useScope: "global" })

const saveChange = inject(saveChangeKey)

// The user named to be granted access without having asked for it, and what naming them ran into (an
// unknown subject, see InputIdentity). Granting them anything makes them a user with access, which is
// when the name is cleared (see onGrantNamedUser).
const namedUser = ref("")
const namedUserErrors = ref<ValidationError[]>([])
const namedUserErrorMessage = computed(() => pickErrorMessage(namedUserErrors.value, t))
const namedUserResolved = computed(() => namedUser.value !== "" && namedUserErrors.value.length === 0)

// What the document grants the named user, which is nothing unless they are listed above already: their
// entry then shows what they hold, so that naming a user who has access reads the same as their entry
// and only what they are missing can be granted here.
const namedUserGrant = computed<PermissionGrant>(() => users.value.find((grant) => grant.user === namedUser.value) ?? { user: namedUser.value, actions: new Map() })

// Both lists are the session document's permission claims, so they follow every change appended to
// the session, whether it was made here, on another tab, or by another client.
const users = computed(() => permissionGrants(props.claims))
const requests = computed(() => permissionRequests(props.claims))

const busy = useBusy()

// ActionCheckbox is one checkbox offered for a user: the action, whether the document grants it to
// them (so the checkbox is checked and toggling it removes the grant), and whether it can be toggled
// at all (see canGrant).
interface ActionCheckbox {
  action: string
  granted: boolean
  toggleable: boolean
}

// actionCheckboxes are the checkboxes offered for the user, ordered the way the permission actions are
// listed: one per action the document grants them (an action outside the permission actions list
// included, so that a grant a client made of its own can be taken away here) and one per action they
// hold through neither the document nor their roles.
function actionCheckboxes(grant: PermissionGrant, identity: Identity): ActionCheckbox[] {
  const actions = new Set(grant.actions.keys())
  for (const { id } of permissionActions) {
    if (!hasUserRoleDocumentPermission(id, { claims: props.claims }, identity.roles)) {
      actions.add(id)
    }
  }
  return [...actions]
    .sort((a, b) => permissionActionOrder(a) - permissionActionOrder(b))
    .map((action) => {
      const granted = grant.actions.has(action)
      return { action, granted, toggleable: granted || canGrant(missingActions(identity, action)) }
    })
}

// actionLabel is the label of an action, falling back to its identifier for an action the permission
// actions list does not cover (nothing stops a client from requesting or claiming any action).
function actionLabel(action: string): string {
  return permissionActionLabel(action, t) ?? action
}

// actionHint describes at more length what an action allows, and is undefined for an action the
// permission actions list does not cover, which has nothing to say beyond its identifier.
function actionHint(action: string): string | undefined {
  return permissionActionHint(action, t) ?? undefined
}

// append runs the changes of one decision, keeping the buttons busy while they are being appended.
async function append(changes: () => Promise<void>): Promise<void> {
  if (!saveChange) {
    return
  }
  busy.value += 1
  try {
    await changes()
  } catch (err) {
    // TODO: Show notification with error.
    console.error("PermissionsForm.append", err)
  } finally {
    busy.value -= 1
  }
}

// grantActions adds a permission claim per action, each with the user and the self scope under it.
async function grantActions(user: string, actions: Iterable<string>): Promise<void> {
  for (const action of actions) {
    const { id: claimID } = await saveChange!({ type: "add", patch: { type: "ref", confidence: HighConfidence, prop: HAS_PERMISSION, to: action } })
    await saveChange!({ type: "add", under: claimID, patch: { type: "id", confidence: HighConfidence, prop: PERMISSION_USER, value: user } })
    await saveChange!({ type: "add", under: claimID, patch: { type: "string", confidence: HighConfidence, prop: PERMISSION_SCOPE, string: SCOPE_SELF } })
  }
}

// TODO: Take access away from one user without taking it from the others the same claim names.
//       A permission claim grants its action to every user its PERMISSION_USER sub-claims name (see
//       permissionGrants), while access is taken away here by removing the whole claim, so a claim
//       naming several users loses it for all of them at once. Nothing this tab or the create-session
//       seeding writes names more than one user, but another client can. Removing only that user's
//       sub-claim would be precise, but a claim which survives a change counts as granted by
//       checkChangePermission in permissions.go, so it would ask the caller to hold the very action
//       they are taking away.
async function removeClaims(claimIDs: Iterable<string>): Promise<void> {
  for (const claimID of claimIDs) {
    await saveChange!({ type: "remove", id: claimID })
  }
}

// claimsOf returns the claims granting the user the given actions.
function claimsOf(user: string, actions: Iterable<string>): string[] {
  const grant = users.value.find((g) => g.user === user)
  return [...actions].flatMap((action) => grant?.actions.get(action) ?? [])
}

// Checking an action grants it, together with what it requires and the user is missing, the same way
// approving a request for it would. Unchecking removes it and everything the document grants which
// requires it: a user is never left with an action without the actions it builds on.
function onToggleAction(grant: PermissionGrant, identity: Identity, action: string) {
  const current = new Set(grant.actions.keys())
  if (!current.has(action)) {
    void append(() => grantActions(grant.user, missingActions(identity, action)))
    return
  }
  const kept = permissionActionsWithout(current, action)
  void append(() =>
    removeClaims(
      claimsOf(
        grant.user,
        [...current].filter((a) => !kept.has(a)),
      ),
    ),
  )
}

function onRemoveUser(grant: PermissionGrant) {
  void append(() => removeClaims([...grant.actions.values()].flat()))
}

// Granting an action to a named user makes them a user with access, listed with everybody else, so
// naming them is done: the name is cleared once the grant has landed, ready for the next user.
function onGrantNamedUser(identity: Identity, action: string) {
  void append(async () => {
    await grantActions(identity.subject, missingActions(identity, action))
    namedUser.value = ""
  })
}

// grantedSet is what the document grants the user, which is what the tab can add to and take from.
function grantedSet(user: string): Set<string> {
  return new Set(users.value.find((grant) => grant.user === user)?.actions.keys() ?? [])
}

// missingActions are the actions to grant the user so that the given one makes sense for them: the
// action itself, and the actions it requires which they hold neither through the document nor through
// their roles (granting those would add nothing). The action itself is granted even when their roles
// already cover it, so that the grant is recorded and can be taken back here.
function missingActions(identity: Identity, action: string): string[] {
  const granted = grantedSet(identity.subject)
  return [...permissionActionClosure(action)].filter(
    (required) => !granted.has(required) && (required === action || !hasUserDocumentPermission(required, { claims: props.claims }, identity.subject, identity.roles)),
  )
}

// canGrant reports whether the caller may grant the actions: the server refuses a change granting an
// action the caller does not hold themselves, so an action they lack cannot be granted from here to
// anyone. Only granting is checked, because taking access away is not granting.
function canGrant(actions: Iterable<string>): boolean {
  return [...actions].every((action) => hasDocumentPermission(action, { claims: props.claims }))
}

// Approving a request grants what the user is missing for it and removes the request. Pending requests
// of theirs for any of the granted actions are approved along: they ask for access which is being
// granted right now, so deciding them again would say nothing.
function onApprove(identity: Identity, request: PermissionRequest) {
  const missing = missingActions(identity, request.action)
  const decided = new Set([request.action, ...missing])
  const approved = requests.value.filter((other) => other.user === request.user && decided.has(other.action))
  void append(async () => {
    await grantActions(request.user, missing)
    await removeClaims(approved.map((other) => other.claimID))
  })
}

function onDeny(request: PermissionRequest) {
  void append(() => removeClaims([request.claimID]))
}

const WithDocumentIdentity = WithDocument<Identity>
</script>

<template>
  <div class="flex flex-col gap-y-4">
    <div>
      <h2 class="text-xl font-bold">{{ t("partials.PermissionsForm.usersTitle") }}</h2>
      <p v-if="users.length === 0" class="mt-1 text-gray-700">{{ t("partials.PermissionsForm.noUsers") }}</p>
      <ul v-else class="mt-2 flex flex-col gap-y-3">
        <li v-for="grant of users" :key="grant.user" class="flex flex-col gap-y-2 rounded border border-slate-300 p-3">
          <!-- The checkboxes need the user's roles, so that an action their roles already cover is not offered. -->
          <WithDocumentIdentity :id="grant.user" name="UserGet">
            <template #default="{ doc: identity }">
              <div class="flex flex-row items-center justify-between gap-4">
                <IdentityLabel :identity="identity" class="font-medium" />
                <!-- TODO: Remove should be shown even on loading error, so that on can remove the request for non-existing user. -->
                <Button type="button" :progress="busy" @click.prevent="onRemoveUser(grant)">{{ t("partials.PermissionsForm.removeAccess") }}</Button>
              </div>
              <div class="flex flex-col gap-y-1">
                <div class="flex flex-row flex-wrap gap-x-6 gap-y-1">
                  <label
                    v-for="checkbox of actionCheckboxes(grant, identity as Identity)"
                    :key="checkbox.action"
                    class="flex items-center gap-x-2"
                    :class="checkbox.toggleable ? 'cursor-pointer' : 'cursor-not-allowed text-gray-500'"
                    :title="actionHint(checkbox.action)"
                  >
                    <CheckBox
                      :model-value="checkbox.granted"
                      :disabled="!checkbox.toggleable"
                      @update:model-value="onToggleAction(grant, identity as Identity, checkbox.action)"
                    />
                    {{ actionLabel(checkbox.action) }}
                  </label>
                </div>
                <!-- Every action their roles cover is left out, so a user whose roles cover them all has no checkbox at all. -->
                <i v-if="actionCheckboxes(grant, identity as Identity).length === 0" class="text-sm text-neutral-500">
                  {{ t("partials.PermissionsForm.nothingToGrant") }}
                </i>
                <i v-else-if="actionCheckboxes(grant, identity as Identity).some((checkbox) => !checkbox.toggleable)" class="text-sm text-neutral-500">
                  {{ t("partials.PermissionsForm.cannotGrant") }}
                </i>
              </div>
            </template>
            <template #loading>
              <i class="text-gray-500">{{ t("common.status.loading") }}</i>
            </template>
          </WithDocumentIdentity>
        </li>
      </ul>
      <!--
        Access can be granted to a user who has not asked for it, by naming them in this entry: it holds
        the same permissions a user with access has, in the same place, and granting any of them makes
        them one of those users, listed above.
      -->
      <div class="mt-3 flex flex-col gap-y-2 rounded border border-slate-300 p-3">
        <div class="flex flex-col gap-y-1">
          <InputIdentity v-model="namedUser" :aria-label="t('partials.PermissionsForm.addUser')" @errors="namedUserErrors = $event" />
          <p v-if="namedUserErrorMessage" class="text-sm text-error-600">{{ namedUserErrorMessage }}</p>
          <p class="text-sm text-neutral-500 italic">{{ t("partials.PermissionsForm.addUserHint") }}</p>
        </div>
        <!-- With nobody named there is nobody to grant anything to: the permissions show what could be granted, and none of them can be. -->
        <div v-if="!namedUserResolved" class="flex flex-row flex-wrap gap-x-6 gap-y-1">
          <label v-for="action of permissionActions" :key="action.id" class="flex cursor-not-allowed items-center gap-x-2 text-gray-500" :title="actionHint(action.id)">
            <CheckBox :model-value="false" disabled />
            {{ actionLabel(action.id) }}
          </label>
        </div>
        <!-- A named user has the permissions read against their roles, like a user with access has. -->
        <WithDocumentIdentity v-else :id="namedUser" name="UserGet">
          <template #default="{ doc: identity }">
            <div class="flex flex-col gap-y-1">
              <div class="flex flex-row flex-wrap gap-x-6 gap-y-1">
                <label
                  v-for="checkbox of actionCheckboxes(namedUserGrant, identity as Identity)"
                  :key="checkbox.action"
                  class="flex items-center gap-x-2"
                  :class="!checkbox.granted && checkbox.toggleable ? 'cursor-pointer' : 'cursor-not-allowed text-gray-500'"
                  :title="actionHint(checkbox.action)"
                >
                  <!-- Access is only granted here: what a user holds already is theirs to lose in their own entry above. -->
                  <CheckBox
                    :model-value="checkbox.granted"
                    :disabled="checkbox.granted || !checkbox.toggleable"
                    @update:model-value="onGrantNamedUser(identity as Identity, checkbox.action)"
                  />
                  {{ actionLabel(checkbox.action) }}
                </label>
              </div>
              <i v-if="actionCheckboxes(namedUserGrant, identity as Identity).length === 0" class="text-sm text-neutral-500">
                {{ t("partials.PermissionsForm.nothingToGrant") }}
              </i>
              <template v-else>
                <!-- The two say different things, so a user who is listed above and holds an action nobody can grant gets both. -->
                <i v-if="actionCheckboxes(namedUserGrant, identity as Identity).some((checkbox) => checkbox.granted)" class="text-sm text-neutral-500">
                  {{ t("partials.PermissionsForm.alreadyGranted") }}
                </i>
                <i v-if="actionCheckboxes(namedUserGrant, identity as Identity).some((checkbox) => !checkbox.toggleable)" class="text-sm text-neutral-500">
                  {{ t("partials.PermissionsForm.cannotGrant") }}
                </i>
              </template>
            </div>
          </template>
          <template #loading>
            <i class="text-gray-500">{{ t("common.status.loading") }}</i>
          </template>
        </WithDocumentIdentity>
      </div>
    </div>
    <div>
      <h2 class="text-xl font-bold">{{ t("partials.PermissionsForm.requestsTitle") }}</h2>
      <p v-if="requests.length === 0" class="mt-1 text-gray-700">{{ t("partials.PermissionsForm.noRequests") }}</p>
      <ul v-else class="mt-2 flex flex-col gap-y-3">
        <!-- One claim can ask on behalf of several users, so it takes both to tell its requests apart. -->
        <li v-for="request of requests" :key="`${request.claimID}-${request.user}`" class="flex flex-col gap-y-2 rounded border border-slate-300 p-3">
          <!--
            The whole request is shown only once the user is known: deciding it has to know what they
            already hold, and whether it can be approved at all is part of what the request says, so a
            user who cannot be looked up leaves their request untouched altogether.
          -->
          <WithDocumentIdentity :id="request.user" name="UserGet">
            <template #default="{ doc: identity }">
              <div class="flex flex-row items-center justify-between gap-4">
                <IdentityLabel :identity="identity" class="font-medium" />
                <div class="flex flex-row gap-x-2">
                  <!-- TODO: Deny should be shown even on loading error, so that on can remove the request for non-existing user. -->
                  <Button type="button" :progress="busy" @click.prevent="onDeny(request)">{{ t("partials.PermissionsForm.deny") }}</Button>
                  <Button
                    type="button"
                    primary
                    :progress="busy"
                    :disabled="!canGrant(missingActions(identity as Identity, request.action))"
                    @click.prevent="onApprove(identity as Identity, request)"
                    >{{ t("partials.PermissionsForm.approve") }}</Button
                  >
                </div>
              </div>
              <div class="flex flex-col gap-y-1">
                <div class="text-gray-700">{{ t("partials.PermissionsForm.requestedAction") }}: {{ actionLabel(request.action) }}</div>
                <div v-if="actionHint(request.action)" class="text-sm text-neutral-500 italic">{{ actionHint(request.action) }}</div>
              </div>
              <div v-if="request.note" class="break-words whitespace-pre-wrap text-gray-700">{{ request.note }}</div>
              <i v-if="!canGrant(missingActions(identity as Identity, request.action))" class="text-sm text-neutral-500">{{
                t("partials.PermissionsForm.cannotGrant")
              }}</i>
            </template>
            <template #loading>
              <i class="text-gray-500">{{ t("common.status.loading") }}</i>
            </template>
          </WithDocumentIdentity>
        </li>
      </ul>
    </div>
  </div>
</template>
