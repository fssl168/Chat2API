#!/usr/bin/env node
/**
 * Chat2API 调试启动脚本 - 交互式菜单
 *
 * 使用方法:
 *   node start.mjs              # 显示交互式菜单
 *   node start.mjs dev          # 开发模式（electron-vite dev）
 *   node start.mjs dev:sandbox  # 开发模式（--no-sandbox，适用于 root 用户）
 *   node start.mjs build        # 构建项目
 *   node start.mjs preview      # 预览构建产物
 *   node start.mjs stop         # 停止所有服务
 *   node start.mjs status       # 查看服务状态
 */

import { spawn, execSync } from 'child_process';
import { createInterface } from 'readline';
import { existsSync } from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// 判断是否为 Windows
const isWindows = process.platform === 'win32';

// 配置
const CONFIG = {
  rendererPort: 5173,
  logLevel: process.env.LOG_LEVEL || 'DEBUG',
  npmRegistry: process.env.NPM_REGISTRY || 'https://registry.npmmirror.com',
};

// 存储子进程
const processes = [];

// 颜色输出
const colors = {
  reset: '\x1b[0m',
  bright: '\x1b[1m',
  dim: '\x1b[2m',
  red: '\x1b[31m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  magenta: '\x1b[35m',
  cyan: '\x1b[36m',
};

const log = {
  info: (msg) => console.log(`${colors.cyan}[INFO]${colors.reset} ${msg}`),
  success: (msg) => console.log(`${colors.green}[OK]${colors.reset} ${msg}`),
  warn: (msg) => console.log(`${colors.yellow}[WARN]${colors.reset} ${msg}`),
  error: (msg) => console.log(`${colors.red}[ERROR]${colors.reset} ${msg}`),
  title: (msg) => console.log(`\n${colors.bright}${colors.magenta}${msg}${colors.reset}`),
};

// 清理并退出
function cleanup() {
  console.log('\n');
  log.info('正在关闭所有服务...');
  processes.forEach(proc => {
    if (proc && !proc.killed) {
      try {
        if (isWindows) {
          execSync(`taskkill /PID ${proc.pid} /T /F`, { stdio: 'ignore', shell: true });
        } else {
          proc.kill('SIGTERM');
        }
      } catch { /* 进程可能已退出 */ }
    }
  });
  log.success('已退出');
  process.exit(0);
}

process.on('SIGINT', cleanup);
process.on('SIGTERM', cleanup);

// 检查命令是否存在
function commandExists(cmd) {
  try {
    execSync(`${isWindows ? 'where' : 'which'} ${cmd}`, { stdio: 'ignore' });
    return true;
  } catch {
    return false;
  }
}

// 检查 Node.js 版本
function getNodeVersion() {
  try {
    return execSync('node --version', { encoding: 'utf-8' }).trim();
  } catch {
    return null;
  }
}

// 检查 npm 依赖是否已安装
function checkDepsInstalled() {
  return existsSync(join(__dirname, 'node_modules'));
}

// 检查构建产物是否存在
function checkBuildOutput() {
  return existsSync(join(__dirname, 'out', 'main', 'index.js'));
}

// 查找占用端口的进程 PID
function findPidByPort(port) {
  try {
    if (isWindows) {
      const output = execSync(`netstat -ano | findstr :${port} | findstr LISTENING`, {
        encoding: 'utf-8',
        shell: true,
        stdio: ['pipe', 'pipe', 'ignore'],
      });
      const pids = new Set();
      for (const line of output.trim().split('\n')) {
        const parts = line.trim().split(/\s+/);
        const pid = parts[parts.length - 1];
        if (pid && pid !== '0') pids.add(pid);
      }
      return [...pids];
    } else {
      const output = execSync(`lsof -ti :${port}`, {
        encoding: 'utf-8',
        stdio: ['pipe', 'pipe', 'ignore'],
      });
      return output.trim().split('\n').filter(Boolean);
    }
  } catch {
    return [];
  }
}

// 获取运行中的服务状态
function getRunningStatus() {
  const rendererPids = findPidByPort(CONFIG.rendererPort);
  return {
    renderer: rendererPids,
    isRunning: rendererPids.length > 0,
  };
}

// 停止服务
async function stopServices() {
  const running = getRunningStatus();

  if (!running.isRunning && processes.length === 0) {
    log.warn('没有检测到正在运行的服务');
    return;
  }

  log.title('========== 停止服务 ==========');

  // 先尝试停止我们启动的子进程
  processes.forEach(proc => {
    if (proc && !proc.killed) {
      try {
        if (isWindows) {
          execSync(`taskkill /PID ${proc.pid} /T /F`, { stdio: 'ignore', shell: true });
        } else {
          proc.kill('SIGTERM');
        }
      } catch { /* 进程可能已退出 */ }
    }
  });
  processes.length = 0;

  // 再通过端口查找并停止残留进程
  const killProcess = async (pid) => {
    try {
      if (isWindows) {
        try {
          execSync(`taskkill /PID ${pid}`, { stdio: 'ignore', shell: true });
        } catch {
          execSync(`taskkill /F /T /PID ${pid}`, { stdio: 'ignore', shell: true });
        }
      } else {
        execSync(`kill -15 ${pid}`, { stdio: 'ignore' });
        await new Promise(r => setTimeout(r, 500));
        try {
          execSync(`kill -0 ${pid}`, { stdio: 'ignore' });
          execSync(`kill -9 ${pid}`, { stdio: 'ignore' });
        } catch { /* 进程已退出 */ }
      }
    } catch { /* 进程可能已退出 */ }
  };

  if (running.renderer.length > 0) {
    log.info(`停止渲染进程 (端口 ${CONFIG.rendererPort}, PID: ${running.renderer.join(', ')})...`);
    for (const pid of running.renderer) await killProcess(pid);
    log.success('渲染进程已停止');
  }

  log.success('所有服务已停止');
}

// 安装依赖
async function installDeps() {
  log.info(`安装依赖 (npm install, registry: ${CONFIG.npmRegistry})...`);
  return new Promise((resolve, reject) => {
    const proc = spawn('npm', ['install', '--registry', CONFIG.npmRegistry], {
      cwd: __dirname,
      stdio: 'inherit',
      shell: true,
    });
    proc.on('close', code => code === 0 ? resolve() : reject(new Error('依赖安装失败')));
  });
}

// 确保依赖已安装
async function ensureDeps() {
  if (!checkDepsInstalled()) {
    log.warn('检测到依赖未安装，正在安装...');
    await installDeps();
  }
}

// 开发模式启动
async function startDev(options = {}) {
  await ensureDeps();

  const electronArgs = [];
  if (options.noSandbox) electronArgs.push('--no-sandbox');
  if (options.disableGpu) electronArgs.push('--disable-gpu', '--disable-software-rasterizer');

  const spawnArgs = ['electron-vite', 'dev'];
  if (electronArgs.length > 0) {
    spawnArgs.push('--', ...electronArgs);
  }

  log.info(`启动开发模式...${electronArgs.length > 0 ? ` (${electronArgs.join(', ')})` : ''}`);
  log.info(`渲染器地址: http://localhost:${CONFIG.rendererPort}`);

  const proc = spawn('npx', spawnArgs, {
    cwd: __dirname,
    stdio: 'inherit',
    shell: true,
    env: {
      ...process.env,
      NODE_ENV: 'development',
    },
  });
  processes.push(proc);
  return proc;
}

// 构建项目
async function buildProject() {
  await ensureDeps();
  log.info('构建项目...');
  return new Promise((resolve, reject) => {
    const proc = spawn('npm', ['run', 'build'], {
      cwd: __dirname,
      stdio: 'inherit',
      shell: true,
    });
    proc.on('close', code => code === 0 ? resolve() : reject(new Error('构建失败')));
  });
}

// 预览构建产物
async function startPreview() {
  if (!checkBuildOutput()) {
    log.warn('未找到构建产物，正在构建...');
    await buildProject();
  }
  log.info('启动预览模式...');
  const proc = spawn('npx', ['electron-vite', 'preview'], {
    cwd: __dirname,
    stdio: 'inherit',
    shell: true,
  });
  processes.push(proc);
  return proc;
}

// 显示状态信息
function showStatus() {
  console.log('\n' + '─'.repeat(50));
  log.success(`渲染器 Dev: http://localhost:${CONFIG.rendererPort}`);
  console.log('─'.repeat(50));
  log.info('按 Ctrl+C 停止所有服务\n');
}

// 等待进程退出
function waitForProcesses() {
  return new Promise(resolve => {
    const check = setInterval(() => {
      const activeCount = processes.filter(proc => proc.exitCode === null && proc.signalCode === null).length;
      if (activeCount === 0) {
        clearInterval(check);
        resolve();
      }
    }, 1000);
  });
}

// 检测当前是否为 root 用户
function isRootUser() {
  if (isWindows) return false;
  try {
    return execSync('id -u', { encoding: 'utf-8' }).trim() === '0';
  } catch {
    return false;
  }
}

// 交互式菜单
async function showMenu() {
  const rl = createInterface({ input: process.stdin, output: process.stdout });
  const question = (prompt) => new Promise(resolve => rl.question(prompt, resolve));

  console.clear();
  log.title('╔══════════════════════════════════════════╗');
  log.title('║       Chat2API 调试启动脚本 (Electron)    ║');
  log.title('╚══════════════════════════════════════════╝');

  // 环境状态
  const nodeVersion = getNodeVersion();
  const depsInstalled = checkDepsInstalled();
  const buildOutput = checkBuildOutput();
  const running = getRunningStatus();
  const rootUser = isRootUser();

  const ok = (v) => v ? `${colors.green}✓${colors.reset}` : `${colors.yellow}✗${colors.reset}`;

  console.log(`\n${colors.bright}环境状态:${colors.reset}`);
  console.log(`  Node.js:     ${nodeVersion ? `${colors.green}${nodeVersion}${colors.reset}` : `${colors.red}未安装${colors.reset}`}`);
  console.log(`  依赖:        ${ok(depsInstalled)} ${depsInstalled ? '已安装' : '未安装'}`);
  console.log(`  构建产物:    ${ok(buildOutput)} ${buildOutput ? '(out/)' : '未构建'}`);
  if (!isWindows) {
    console.log(`  root 用户:   ${rootUser ? `${colors.yellow}是 (需要 --no-sandbox)${colors.reset}` : `${colors.green}否${colors.reset}`}`);
  }

  console.log(`\n${colors.bright}服务状态:${colors.reset}`);
  console.log(`  渲染器 (:${CONFIG.rendererPort}): ${running.renderer.length > 0 ? `${colors.green}运行中${colors.reset} (PID: ${running.renderer.join(', ')})` : `${colors.dim}未运行${colors.reset}`}`);

  console.log(`\n${colors.bright}环境变量:${colors.reset}`);
  console.log(`  LOG_LEVEL:     ${colors.cyan}${CONFIG.logLevel}${colors.reset}`);
  console.log(`  NPM_REGISTRY:  ${colors.cyan}${CONFIG.npmRegistry}${colors.reset}`);
  console.log(`${colors.dim}  自定义: LOG_LEVEL=DEBUG node start.mjs dev${colors.reset}`);

  console.log(`
${colors.bright}请选择操作:${colors.reset}

  ${colors.cyan}1.${colors.reset} 开发模式    (electron-vite dev)
  ${colors.cyan}2.${colors.reset} 开发模式    (无沙箱，适用于 root / WSL)
  ${colors.cyan}3.${colors.reset} 开发模式    (禁用 GPU，解决 GPU 错误)
  ${colors.cyan}4.${colors.reset} 构建项目    (npm run build)
  ${colors.cyan}5.${colors.reset} 预览模式    (electron-vite preview)
  ${colors.cyan}6.${colors.reset} 安装依赖    (npm install)
  ${colors.cyan}7.${colors.reset} 停止所有服务
  ${colors.cyan}0.${colors.reset} 退出
`);

  const choice = await question(`${colors.yellow}请输入选项 [1]: ${colors.reset}`);
  rl.close();

  switch (choice.trim() || '1') {
    case '1':
      log.title('========== 开发模式 ==========');
      await startDev();
      showStatus();
      await waitForProcesses();
      break;

    case '2':
      log.title('========== 开发模式 (无沙箱) ==========');
      await startDev({ noSandbox: true });
      showStatus();
      await waitForProcesses();
      break;

    case '3':
      log.title('========== 开发模式 (禁用 GPU) ==========');
      await startDev({ disableGpu: true });
      showStatus();
      await waitForProcesses();
      break;

    case '4':
      log.title('========== 构建项目 ==========');
      await buildProject();
      log.success('构建完成！');
      break;

    case '5':
      log.title('========== 预览模式 ==========');
      await startPreview();
      showStatus();
      await waitForProcesses();
      break;

    case '6':
      log.title('========== 安装依赖 ==========');
      await installDeps();
      log.success('依赖安装完成！');
      break;

    case '7':
      await stopServices();
      break;

    case '0':
      log.info('再见！');
      process.exit(0);
      break;

    default:
      log.warn('无效选项');
      await showMenu();
  }
}

// 命令行参数处理
async function main() {
  const cmd = process.argv[2];

  switch (cmd) {
    case 'dev':
      log.title('========== 开发模式 ==========');
      await startDev();
      showStatus();
      await waitForProcesses();
      break;

    case 'dev:sandbox':
      log.title('========== 开发模式 (无沙箱) ==========');
      await startDev({ noSandbox: true });
      showStatus();
      await waitForProcesses();
      break;

    case 'dev:nogpu':
      log.title('========== 开发模式 (禁用 GPU) ==========');
      await startDev({ disableGpu: true });
      showStatus();
      await waitForProcesses();
      break;

    case 'build':
      log.title('========== 构建项目 ==========');
      await buildProject();
      log.success('构建完成！');
      break;

    case 'preview':
      log.title('========== 预览模式 ==========');
      await startPreview();
      showStatus();
      await waitForProcesses();
      break;

    case 'install':
      log.title('========== 安装依赖 ==========');
      await installDeps();
      log.success('依赖安装完成！');
      break;

    case 'stop':
      await stopServices();
      break;

    case 'status': {
      const status = getRunningStatus();
      const nodeVer = getNodeVersion();
      console.log(`\n${colors.bright}环境:${colors.reset}`);
      console.log(`  Node.js: ${nodeVer || `${colors.red}未安装${colors.reset}`}`);
      console.log(`  依赖: ${checkDepsInstalled() ? `${colors.green}已安装${colors.reset}` : `${colors.yellow}未安装${colors.reset}`}`);
      console.log(`  构建产物: ${checkBuildOutput() ? `${colors.green}已构建${colors.reset}` : `${colors.yellow}未构建${colors.reset}`}`);
      console.log(`\n${colors.bright}服务状态:${colors.reset}`);
      console.log(`  渲染器 (:${CONFIG.rendererPort}): ${status.renderer.length > 0 ? `${colors.green}运行中${colors.reset} (PID: ${status.renderer.join(', ')})` : `${colors.dim}未运行${colors.reset}`}\n`);
      break;
    }

    case 'help':
    case '-h':
    case '--help':
      console.log(`
${colors.bright}Chat2API 调试启动脚本 (Electron)${colors.reset}

${colors.cyan}使用方法:${colors.reset}
  node start.mjs                显示交互式菜单
  node start.mjs dev            开发模式 (electron-vite dev)
  node start.mjs dev:sandbox    开发模式 (无沙箱，root/WSL 环境)
  node start.mjs dev:nogpu      开发模式 (禁用 GPU，解决 GPU 错误)
  node start.mjs build          构建项目 (npm run build)
  node start.mjs preview        预览构建产物 (electron-vite preview)
  node start.mjs install        安装依赖 (npm install)
  node start.mjs stop           停止所有服务
  node start.mjs status         查看服务状态

${colors.cyan}常用环境变量:${colors.reset}
  LOG_LEVEL          日志级别: DEBUG|INFO|WARN|ERROR (默认: DEBUG)
  NPM_REGISTRY       npm 镜像源 (默认: https://registry.npmmirror.com)

${colors.cyan}示例:${colors.reset}
  node start.mjs dev
  LOG_LEVEL=DEBUG node start.mjs dev
  NPM_REGISTRY=https://registry.npmjs.org node start.mjs install
`);
      break;

    default:
      await showMenu();
  }
}

main().catch(e => {
  log.error(e.message);
  process.exit(1);
});
