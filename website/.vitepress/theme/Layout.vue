<script setup>
import DefaultTheme from 'vitepress/theme'
import { useData } from 'vitepress'
import HeroSection from './HeroSection.vue'

const { Layout } = DefaultTheme
const { frontmatter } = useData()
</script>

<template>
  <Layout>
    <template #home-hero-before>
      <HeroSection v-if="frontmatter.customHero" />
    </template>
  </Layout>
</template>
