import { createRouter, createWebHashHistory } from 'vue-router'
import Home from '../views/home/Home.vue'
import AddServer from '../views/addServer/AddServer.vue'
import Remote from '../views/remote/Remote.vue'
import RemoteControl from '../views/remote/RemoteControl.vue'
import Promotion from '../views/promotion/Promotion.vue'

const routes = [
  {
    path: '/',
    name: 'home',
    component: Home
  },
  {
    path: '/servers/add',
    name: 'add-server',
    component: AddServer
  },
  {
    path: '/promotion',
    name: 'promotion',
    component: Promotion
  },
  {
    path: '/servers/:id/edit',
    name: 'edit-server',
    component: AddServer,
    props: true
  },
  {
    path: '/remote/:id',
    name: 'server',
    component: Remote,
    props: true
  },
  {
    path: '/remote/:id/device/:deviceId',
    name: 'remote-control',
    component: RemoteControl,
    props: true,
    meta: { standalone: true }
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/'
  }
]

export default createRouter({
  history: createWebHashHistory(),
  routes
})
