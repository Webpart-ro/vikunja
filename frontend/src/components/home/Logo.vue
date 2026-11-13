<script setup lang="ts">
import { computed } from 'vue'
import { useColorScheme } from '@/composables/useColorScheme'

// Webpart branding replaces the Vikunja wordmark (incl. the June pride variant).
import webpartLogoBlack from '@/assets/webpart-logo-black.png'
import webpartLogoWhite from '@/assets/webpart-logo-white.png'

const { isDark } = useColorScheme()

const CustomLogo = computed(() => {
	const lightLogo = window.CUSTOM_LOGO_URL
	const darkLogo = window.CUSTOM_LOGO_URL_DARK

	if (!lightLogo && !darkLogo) return ''
	if (!darkLogo) return lightLogo
	if (!lightLogo) return darkLogo

	return isDark.value ? darkLogo : lightLogo
})
</script>

<template>
	<div>
		<img
			v-if="!CustomLogo"
			:src="isDark ? webpartLogoWhite : webpartLogoBlack"
			alt="Webpart"
			class="logo"
		>
		<img
			v-else
			:src="CustomLogo"
			alt="Webpart"
			class="logo"
		>
	</div>
</template>

<style lang="scss" scoped>
.logo {
	color: var(--logo-text-color);
	inline-size: 168px;
	max-inline-size: 100%;
	block-size: auto;
}
</style>
