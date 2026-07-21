'use strict';
const fs = require('fs');
const path = require('path');

const directory = JSON.parse(fs.readFileSync(path.join(__dirname, 'directory.json'), 'utf8'));

function evaluatePrequester(userId, projectId) {
    const user = directory[userId];
    if (!user) return { ok: false, reason: "Utilisateur inconnu dans l'annuaire", clearance: null };
    if (!user.active) return { ok: false, reason: "Compte utilisateur inactif", clearance: null };
    if (!user.projects.includes(projectId)) return { ok: false, reason: `Non autorisé sur le projet ${projectId}`, clearance: null };
    
    // Si tout est bon, on retourne l'habilitation réelle de l'utilisateur
    return { ok: true, reason: null, clearance: user.clearance };
}

module.exports = { evaluatePrequester };
