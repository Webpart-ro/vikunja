<template>
	<div class="profile-photo">
		<UserAvatar
			:user="authStore.info"
			:size="72"
			:alt="$t('webpart.profilePhoto.current')"
			class="profile-photo__avatar"
		/>
		<div class="profile-photo__actions">
			<template v-if="isSynced">
				<p class="help">
					{{ $t('webpart.profilePhoto.synced') }}
				</p>
			</template>
			<template v-else>
				<input
					ref="fileInput"
					accept="image/*"
					class="is-hidden"
					type="file"
					@change="onFilePicked"
				>
				<div class="profile-photo__buttons">
					<XButton
						:loading="busy"
						icon="cloud-upload-alt"
						@click="fileInput?.click()"
					>
						{{ hasUpload ? $t('webpart.profilePhoto.change') : $t('webpart.profilePhoto.upload') }}
					</XButton>
					<XButton
						v-if="hasUpload"
						variant="tertiary"
						:disabled="busy"
						@click="removePhoto"
					>
						{{ $t('webpart.profilePhoto.remove') }}
					</XButton>
				</div>
				<p class="help">
					{{ $t('webpart.profilePhoto.hint') }}
					<RouterLink :to="{name: 'user.settings.avatar'}">
						{{ $t('webpart.profilePhoto.moreOptions') }}
					</RouterLink>
				</p>
			</template>
		</div>
	</div>
</template>

<script setup lang="ts">
import {computed, ref} from 'vue'
import {useQuery} from '@tanstack/vue-query'

import UserAvatar from '@/components/misc/UserAvatar.vue'
import {avatarProviderQuery, useUpdateAvatarProviderMutation, useUploadAvatarMutation} from '@/client/queries/avatars'
import {useAuthStore} from '@/stores/auth'
import {error} from '@/message'

// Big enough for any avatar size the app asks for, small enough to upload fast.
const OUTPUT_SIZE = 512

const authStore = useAuthStore()
const providerQuery = useQuery(avatarProviderQuery())
const upload = useUploadAvatarMutation()
const updateProvider = useUpdateAvatarProviderMutation()

const provider = computed(() => providerQuery.data.value?.avatar_provider)
const isSynced = computed(() => provider.value === 'ldap' || provider.value === 'openid')
const hasUpload = computed(() => provider.value === 'upload')
const preparing = ref(false)
const busy = computed(() => preparing.value || upload.isPending.value || updateProvider.isPending.value)

const fileInput = ref<HTMLInputElement>()

/** Center-crops the picked image to a square so faces are not squashed. */
async function squareCrop(file: File): Promise<Blob | null> {
	const bitmap = await createImageBitmap(file)
	const side = Math.min(bitmap.width, bitmap.height)
	const size = Math.min(side, OUTPUT_SIZE)
	const canvas = document.createElement('canvas')
	canvas.width = size
	canvas.height = size
	canvas.getContext('2d')?.drawImage(
		bitmap,
		(bitmap.width - side) / 2,
		(bitmap.height - side) / 2,
		side,
		side,
		0,
		0,
		size,
		size,
	)
	bitmap.close()
	return new Promise(resolve => canvas.toBlob(resolve, 'image/png'))
}

async function onFilePicked() {
	const file = fileInput.value?.files?.[0]
	const username = authStore.info?.username
	if (!file || !username) return

	preparing.value = true
	try {
		const blob = await squareCrop(file)
		if (blob) await upload.mutateAsync({username, blob})
	} catch (e) {
		error(e)
	} finally {
		preparing.value = false
		if (fileInput.value) fileInput.value.value = ''
	}
}

function removePhoto() {
	const username = authStore.info?.username
	if (!username) return
	updateProvider.mutate({username, provider: 'initials'})
}
</script>

<style lang="scss" scoped>
.profile-photo {
	display: flex;
	align-items: center;
	gap: 1rem;
}

.profile-photo__avatar {
	inline-size: 72px;
	block-size: 72px;
	border-radius: 100%;
	flex-shrink: 0;
	object-fit: cover;
	box-shadow: 0 0 0 3px var(--white), 0 0 0 4px var(--grey-200);
}

.profile-photo__buttons {
	display: flex;
	flex-wrap: wrap;
	gap: .5rem;
}

.profile-photo__actions .help {
	margin-block-start: .5rem;
}
</style>
