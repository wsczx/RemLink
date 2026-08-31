<template>
  <div class="not-found-page">
    <div class="nf-bg">
      <div class="bg-circle bg-circle-1"></div>
      <div class="bg-circle bg-circle-2"></div>
      <div class="bg-circle bg-circle-3"></div>
    </div>
    <div class="nf-card">
      <div class="nf-code">404</div>
      <div class="nf-title">页面不存在</div>
      <div class="nf-desc">您访问的页面不存在或已被移动。</div>
      <div class="nf-actions">
        <el-button type="primary" @click="goHome">返回首页</el-button>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'NotFound',
  methods: {
    goHome() {
      window.location.href = '/'
    },
  },
}
</script>

<style scoped>
.not-found-page {
  position: relative;
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  background: linear-gradient(135deg, #1b2138 0%, #2a3a5c 40%, #1a3668 100%);
}

/* 背景装饰圆，与登录页保持一致 */
.nf-bg {
  position: absolute;
  inset: 0;
  pointer-events: none;
}
.bg-circle {
  position: absolute;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.03);
}
.bg-circle-1 {
  width: 500px;
  height: 500px;
  top: -150px;
  right: -100px;
}
.bg-circle-2 {
  width: 300px;
  height: 300px;
  bottom: -80px;
  left: -60px;
}
.bg-circle-3 {
  width: 200px;
  height: 200px;
  bottom: 30%;
  right: 15%;
  background: rgba(64, 158, 255, 0.06);
}

.nf-card {
  position: relative;
  z-index: 1;
  text-align: center;
  padding: 48px 40px;
  background: var(--bg-card);
  border-radius: 16px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.08);
  max-width: 420px;
}
.nf-code {
  font-size: 96px;
  font-weight: 800;
  line-height: 1;
  color: var(--color-primary);
}
.nf-title {
  font-size: 22px;
  font-weight: 600;
  margin-top: 12px;
  color: var(--text-primary);
}
.nf-desc {
  font-size: 14px;
  color: var(--text-secondary);
  margin-top: 8px;
}
.nf-actions {
  margin-top: 28px;
}
</style>
