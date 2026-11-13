<script setup lang="ts">
import {computed} from 'vue'

import type {Label} from '@/client/generated'
import {getLabelColor} from '@/composables/useLabelStyles'

const props = defineProps<{
	label: Label
}>()

// Webpart: soft neutral chip with a coloured dot instead of a fully coloured pill,
// so arbitrary label colours stay readable in light and dark mode.
const dotStyle = computed(() => ({
	background: getLabelColor(props.label) || 'var(--grey-400)',
}))
</script>

<template>
	<span
		:key="label.id"
		class="tag label-chip"
	>
		<span
			class="label-chip__dot"
			:style="dotStyle"
			aria-hidden="true"
		/>
		<span>{{ label.title }}</span>
	</span>
</template>

<style lang="scss" scoped>
.label-chip {
	display: inline-flex;
	align-items: center;
	gap: .35rem;
	block-size: auto;
	padding: .1rem .5rem;
	border-radius: var(--radius-sm);
	border: 1px solid var(--grey-200);
	background: var(--grey-100);
	color: var(--grey-700);
	font-size: .75rem;
	font-weight: 500;
	line-height: 1.5;
}

.label-chip__dot {
	display: inline-block;
	inline-size: .5rem !important;
	block-size: .5rem;
	min-inline-size: .5rem;
	border-radius: 50%;
	flex-shrink: 0;
}
</style>
