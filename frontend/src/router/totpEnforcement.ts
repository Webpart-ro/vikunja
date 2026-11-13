import type {RouteLocationNormalized, RouteLocationRaw} from 'vue-router'

import {queryClient} from '@/client/queryClient'
import {totpQuery} from '@/client/queries/totp'
import {loadUserInfo} from '@/router/loadUserInfo'
import {useConfigStore} from '@/stores/config'

// Webpart: mandatory 2FA. When the server enforces TOTP (service.enforcetotp), a local
// account without it is kept on the TOTP settings page; the API rejects everything else
// with code 1090 anyway, this just sends people to the one page that works.

const TOTP_ROUTE = 'user.settings.totp'

// Users whose TOTP we've already seen enabled this session; avoids a request per navigation.
const confirmedUserIds = new Set<number>()

export async function totpEnrollmentRedirect(to: RouteLocationNormalized): Promise<RouteLocationRaw | undefined> {
	if (to.name === TOTP_ROUTE) return

	const configStore = useConfigStore()
	if (!configStore.totp_enabled || !configStore.totp_enforced) return

	const info = await loadUserInfo()
	if (!info?.id || !info.is_local_user || info.bot_owner_id) return
	if (confirmedUserIds.has(info.id)) return

	try {
		const totp = await queryClient.fetchQuery(totpQuery())
		if (totp.enabled) {
			confirmedUserIds.add(info.id)
			return
		}
	} catch {
		// Can't tell (network etc.): let the navigation through, the API still enforces it.
		return
	}

	return {name: TOTP_ROUTE}
}
