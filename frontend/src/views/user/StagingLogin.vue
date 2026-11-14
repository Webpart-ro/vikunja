<template>
	<div class="staging-login">
		<Message
			v-if="errorMessage"
			variant="danger"
		>
			{{ errorMessage }}
		</Message>
		<Message v-else>
			{{ $t('webpart.staging.opening', {host}) }}
		</Message>
	</div>
</template>

<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'
import {useRoute} from 'vue-router'
import {useI18n} from 'vue-i18n'
import {useTitle} from '@vueuse/core'

import Message from '@/components/misc/Message.vue'
import {getErrorText} from '@/message'
import {stagingAuthSession} from '@/client/generated'

// Webpart staging single sign-on: staging websites send logged-out browsers here
// (?rd=<staging URL>). Being on this page means the user is logged in (and 2FA is done),
// so ask the API for the shared staging cookie and go back.
defineOptions({name: 'StagingLogin'})

const {t} = useI18n({useScope: 'global'})
useTitle(() => t('webpart.staging.title'))

const route = useRoute()
const errorMessage = ref('')

const target = computed(() => typeof route.query.rd === 'string' ? route.query.rd : '')
const host = computed(() => {
	try {
		return new URL(target.value).host
	} catch {
		return ''
	}
})

async function startSession() {
	if (!host.value) {
		errorMessage.value = t('webpart.staging.missingTarget')
		return
	}
	try {
		const {data} = await stagingAuthSession({body: {redirect: target.value}})
		window.location.replace(data.redirect || target.value)
	} catch (e) {
		errorMessage.value = getErrorText(e)
	}
}

onMounted(() => startSession())
</script>

<style lang="scss" scoped>
.staging-login {
	max-inline-size: 32rem;
	margin: 3rem auto;
}
</style>
