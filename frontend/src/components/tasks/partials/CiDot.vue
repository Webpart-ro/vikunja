<template>
	<!-- Webpart GitHub integration: CI status of a branch or pull request, linking to the run -->
	<a
		v-if="ci.url"
		v-tooltip="label"
		:href="ci.url"
		target="_blank"
		rel="noopener"
		class="ci-dot"
		:class="`is-${ci.state}`"
		:aria-label="label"
		@click.stop
	>
		<Icon :icon="icon" />
	</a>
	<span
		v-else
		v-tooltip="label"
		class="ci-dot"
		:class="`is-${ci.state}`"
		:aria-label="label"
	>
		<Icon :icon="icon" />
	</span>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useI18n} from 'vue-i18n'

import type {CiStatus} from '@/client/generated'

const props = defineProps<{
	ci: CiStatus
}>()

const {t} = useI18n()

const icon = computed(() => {
	switch (props.ci.state) {
		case 'success': return 'circle-check'
		case 'failure': return 'circle-xmark'
		default: return 'circle-half-stroke'
	}
})

const label = computed(() => t(`webpart.github.ci.${props.ci.state || 'neutral'}`))
</script>

<style lang="scss" scoped>
.ci-dot {
	flex-shrink: 0;
	display: inline-flex;
	align-items: center;
	justify-content: center;
	inline-size: 1.25rem;
	block-size: 1.25rem;
	font-size: .8rem;
	border-radius: 50%;
	color: var(--grey-400);

	&.is-success {
		color: hsl(142, 64%, 34%);
	}

	&.is-failure {
		color: var(--danger-text);
	}

	&.is-pending {
		color: hsl(38, 92%, 40%);
	}
}
</style>
