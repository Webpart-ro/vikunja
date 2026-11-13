<template>
	<div
		v-if="isDone"
		class="is-done"
		:class="{ 'is-done--small': variant === 'small' }"
	>
		{{ $t('task.attributes.done') }}
	</div>
</template>

<script lang="ts" setup>
withDefaults(defineProps<{
	isDone?: boolean
	variant?: 'default' | 'small'
}>(), {
	isDone: false,
	variant: 'default',
})
</script>


<style lang="scss" scoped>
.is-done {
  // Webpart: soft status pill instead of a loud green block
  display: inline-flex;
  align-items: center;
  background: hsla(var(--success-h), var(--success-s), 40%, .14);
  color: var(--success-text);
  padding: .3rem .55rem;
  font-size: .75rem;
  font-weight: 600;
  line-height: 1;
  border-radius: 999px;
  text-align: center;
}

.is-done--small {
	padding: .15rem .45rem;
	font-size: .7rem;
}
</style>
