<template>
	<div class="no-auth-wrapper">
		<Logo
			class="logo"
			width="200"
			height="58"
		/>
		<div class="noauth-container">
			<section
				class="image"
				:class="{ 'has-message': motd !== '' }"
			>
				<Message v-if="motd !== ''">
					{{ motd }}
				</Message>
				<!-- Webpart: branded panel instead of the Vikunja llama photo -->
				<WebpartMark
					class="image-mark"
					aria-hidden="true"
				/>
				<div>
					<p class="image-eyebrow">
						Webpart Tasks
					</p>
					<h2 class="image-title">
						{{ $t("misc.welcomeBack") }}
					</h2>
				</div>
			</section>
			<main
				id="main-content"
				tabindex="-1"
				class="content"
			>
				<div>
					<h2
						v-if="title"
						class="title"
					>
						{{ title }}
					</h2>
					<ApiConfig v-if="shouldShowApiConfig" />
					<Message
						v-if="motd !== ''"
						class="is-hidden-tablet mbe-4"
					>
						{{ motd }}
					</Message>
					<slot />
				</div>
				<Legal />
			</main>
		</div>
	</div>
</template>

<script lang="ts" setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'

import Logo from '@/components/home/Logo.vue'
import WebpartMark from '@/assets/webpart-mark.svg?component'
import Message from '@/components/misc/Message.vue'
import Legal from '@/components/misc/Legal.vue'
import ApiConfig from '@/components/misc/ApiConfig.vue'

import { useTitle } from '@/composables/useTitle'
import { useConfigStore } from '@/stores/config'
import { isDesktopApp } from '@/helpers/desktopAuth'

const props = withDefaults(
	defineProps<{
		showApiConfig?: boolean;
	}>(),
	{
		showApiConfig: false,
	},
)

const isDesktop = isDesktopApp()
const hasStoredApiUrl = isDesktop && localStorage.getItem('API_URL') !== null
// Webpart: frontend and API are served from the same origin, so the web build never shows the API-URL box
const shouldShowApiConfig = computed(() => props.showApiConfig && isDesktop && hasStoredApiUrl)

const configStore = useConfigStore()
const motd = computed(() => configStore.motd)

const route = useRoute()
const { t } = useI18n({ useScope: 'global' })
const title = computed(() =>
	route.meta?.title ? t(route.meta.title as string) : '',
)
useTitle(() => title.value)
</script>

<style lang="scss" scoped>
.no-auth-wrapper {
	background: var(--site-background);
	min-block-size: 100vh;
	display: flex;
	flex-direction: column;
	place-items: center;

	@media screen and (max-width: $fullhd) {
		padding-block-end: 15rem;
	}
}

.noauth-container {
	max-inline-size: $desktop;
	inline-size: 100%;
	min-block-size: 60vh;
	display: flex;
	background-color: var(--white);
	box-shadow: var(--shadow-md);
	overflow: hidden;

	@media screen and (min-width: $desktop) {
		border-radius: $radius;
	}
}

.image {
	inline-size: 50%;
	padding: 1rem;
	display: flex;
	flex-direction: column;
	justify-content: flex-end;

	@media screen and (max-width: $tablet) {
		display: none;
	}

	@media screen and (min-width: $tablet) {
		background:
			radial-gradient(circle at 85% 15%, hsla(15, 83%, 46%, .35), transparent 45%),
			radial-gradient(circle at 10% 90%, hsla(165, 50%, 35%, .55), transparent 50%),
			#10312b;
		position: relative;
		justify-content: space-between;
		padding: 2rem;

		&.has-message {
			justify-content: space-between;
		}

		// Darken mainly the lower part of the photo where the white heading sits so
		// the text keeps a reliable contrast ratio across the whole image.
		// faint grid texture
		&::before {
			content: "";
			position: absolute;
			inset: 0;
			background-image:
				linear-gradient(hsla(0, 0%, 100%, .04) 1px, transparent 1px),
				linear-gradient(90deg, hsla(0, 0%, 100%, .04) 1px, transparent 1px);
			background-size: 32px 32px;
		}

		> * {
			position: relative;
		}
	}
}

.content {
	display: flex;
	justify-content: space-between;
	flex-direction: column;
	padding: 2rem 2rem 1.5rem;

	@media screen and (max-width: $desktop) {
		inline-size: 100%;
		max-inline-size: 450px;
		margin-inline: auto;
	}

	@media screen and (min-width: $desktop) {
		inline-size: 50%;
	}
}

.logo {
	max-inline-size: 100%;
	margin: 1rem 0;
}

.image-mark {
	inline-size: 4rem;
	block-size: 4rem;
	border-radius: 15px;
	box-shadow: 0 0 0 1px hsla(0, 0%, 100%, .15);
}

.image-eyebrow {
	margin: 0 0 .25rem;
	color: hsla(0, 0%, 100%, .7);
	font-size: .85rem;
	font-weight: 600;
	letter-spacing: .08em;
	text-transform: uppercase;
}

.image-title {
	margin: 0;
	color: hsl(0deg, 0%, 100%);
	font-size: 2.25rem;
	font-weight: 700;
	letter-spacing: -0.02em;
}
</style>
